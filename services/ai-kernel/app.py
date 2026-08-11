"""Internal Semantic Kernel boundary for structured extraction.

The service is intentionally stateless and has no database access. The Go
application supplies one short-lived, decrypted provider configuration per
request over the private Compose network. Prompts, credentials and model
responses are never logged here.
"""

from __future__ import annotations

import asyncio
import json
import os
import secrets
from typing import Any, Literal

import openai
import jsonschema
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from openai import AsyncOpenAI
from pydantic import AnyHttpUrl, BaseModel, ConfigDict, Field, SecretStr
from semantic_kernel.connectors.ai.chat_completion_client_base import (
    ChatCompletionClientBase,
)
from semantic_kernel.connectors.ai.prompt_execution_settings import (
    PromptExecutionSettings,
)
from semantic_kernel.contents.chat_history import ChatHistory
from semantic_kernel.contents.chat_message_content import ChatMessageContent
from semantic_kernel.contents.utils.author_role import AuthorRole

PROTOCOL_VERSION = 1
INTERNAL_TOKEN = os.environ.get("AI_KERNEL_INTERNAL_TOKEN", "")

app = FastAPI(
    title="Anby Wiki AI Kernel",
    version="1",
    docs_url=None,
    redoc_url=None,
    openapi_url=None,
)


class RuntimeConfig(BaseModel):
    model_config = ConfigDict(extra="forbid")

    provider: Literal["openai-compatible", "deepseek"]
    base_url: AnyHttpUrl
    api_key: SecretStr
    model: str = Field(min_length=1, max_length=256)
    response_format: Literal["json_object", "json_schema"]
    request_timeout_seconds: int = Field(ge=5, le=300)
    max_attempts: int = Field(ge=1, le=5)


class GenerateRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    version: Literal[1]
    config: RuntimeConfig
    system_prompt: str = Field(min_length=1)
    user_prompt: str = Field(min_length=1)
    json_schema: dict[str, Any]


class OpenAICompatibleExecutionSettings(PromptExecutionSettings):
    """Execution settings carried through the Semantic Kernel boundary."""

    temperature: float = 0
    response_format: dict[str, Any] = Field(default_factory=dict)
    extra_body: dict[str, Any] | None = None


class OpenAICompatibleChatCompletion(ChatCompletionClientBase):
    """Small SK connector for providers implementing the OpenAI chat API.

    Semantic Kernel's bundled OpenAI package imports every optional connector,
    including Azure and realtime media dependencies. Anby only needs chat
    completion, so this service implements the documented SK abstraction while
    delegating transport to the official OpenAI client.
    """

    client: Any = Field(exclude=True)

    def get_prompt_execution_settings_class(
        self,
    ) -> type[OpenAICompatibleExecutionSettings]:
        return OpenAICompatibleExecutionSettings

    async def _inner_get_chat_message_contents(
        self,
        chat_history: ChatHistory,
        settings: PromptExecutionSettings,
    ) -> list[ChatMessageContent]:
        if not isinstance(settings, OpenAICompatibleExecutionSettings):
            settings = OpenAICompatibleExecutionSettings.from_prompt_execution_settings(
                settings
            )

        request: dict[str, Any] = {
            "model": self.ai_model_id,
            "messages": [
                {"role": message.role.value, "content": message.content or ""}
                for message in chat_history.messages
            ],
            "temperature": settings.temperature,
            "response_format": settings.response_format,
        }
        if settings.extra_body:
            request["extra_body"] = settings.extra_body

        response = await self.client.chat.completions.create(**request)
        choice = response.choices[0] if response.choices else None
        content = choice.message.content if choice is not None else None
        return [
            ChatMessageContent(
                role=AuthorRole.ASSISTANT,
                content=content or "",
                inner_content=response,
                ai_model_id=self.ai_model_id,
                metadata={"usage": response.usage},
            )
        ]


def error_response(status: int, code: str, message: str, temporary: bool) -> JSONResponse:
    return JSONResponse(
        status_code=status,
        content={"code": code, "message": message, "temporary": temporary},
    )


def authorized(token: str | None) -> bool:
    return bool(INTERNAL_TOKEN) and bool(token) and secrets.compare_digest(token, INTERNAL_TOKEN)


@app.get("/healthz")
async def healthz() -> dict[str, Any]:
    return {"status": "ok", "version": PROTOCOL_VERSION}


@app.middleware("http")
async def internal_authentication(request: Request, call_next: Any) -> JSONResponse:
    if request.url.path == "/v1/generate" and not authorized(
        request.headers.get("X-Anby-Internal-Token")
    ):
        return error_response(401, "unauthorized", "internal authentication failed", False)
    return await call_next(request)


@app.post("/v1/generate")
async def generate(payload: GenerateRequest) -> JSONResponse:
    config = payload.config
    client = AsyncOpenAI(
        api_key=config.api_key.get_secret_value(),
        base_url=str(config.base_url).rstrip("/"),
        timeout=float(config.request_timeout_seconds),
        max_retries=0,
    )
    service = OpenAICompatibleChatCompletion(
        ai_model_id=config.model,
        service_id="anby-extraction",
        client=client,
    )
    history = ChatHistory(system_message=payload.system_prompt)
    history.add_user_message(payload.user_prompt)

    response_format: dict[str, Any]
    if config.response_format == "json_schema":
        response_format = {
            "type": "json_schema",
            "json_schema": {
                "name": "anby_structured_output",
                "strict": True,
                "schema": payload.json_schema,
            },
        }
    else:
        response_format = {"type": "json_object"}

    settings = OpenAICompatibleExecutionSettings(
        service_id="anby-extraction",
        temperature=0,
        response_format=response_format,
        extra_body={"thinking": {"type": "disabled"}}
        if config.provider == "deepseek"
        else None,
    )

    try:
        for attempt in range(config.max_attempts):
            try:
                message = await asyncio.wait_for(
                    service.get_chat_message_content(chat_history=history, settings=settings),
                    timeout=float(config.request_timeout_seconds) + 5,
                )
                if message is None or not message.content:
                    raise ValueError("empty response")
                document = json.loads(message.content)
                if not isinstance(document, dict):
                    raise ValueError("JSON root is not an object")
                jsonschema.validate(instance=document, schema=payload.json_schema)
                usage = (message.metadata or {}).get("usage")
                input_tokens = int(getattr(usage, "prompt_tokens", 0) or 0)
                output_tokens = int(getattr(usage, "completion_tokens", 0) or 0)
                return JSONResponse(
                    status_code=200,
                    content={
                        "version": PROTOCOL_VERSION,
                        "provider": config.provider,
                        "model": config.model,
                        "json": document,
                        "input_tokens": input_tokens,
                        "output_tokens": output_tokens,
                    },
                )
            except jsonschema.SchemaError:
                return error_response(500, "invalid_schema", "structured output schema is invalid", False)
            except (json.JSONDecodeError, jsonschema.ValidationError, TypeError, ValueError):
                if attempt + 1 >= config.max_attempts:
                    return error_response(502, "invalid_structured_output", "model output failed schema validation", False)
                if message is not None and message.content:
                    history.add_assistant_message(message.content)
                history.add_user_message(
                    "The previous response was invalid. Return a corrected JSON object that conforms exactly to the supplied schema."
                )
            except (asyncio.TimeoutError, openai.APITimeoutError):
                if attempt + 1 >= config.max_attempts:
                    return error_response(504, "timeout", "model request timed out", True)
                await asyncio.sleep(2**attempt)
            except openai.RateLimitError:
                if attempt + 1 >= config.max_attempts:
                    return error_response(429, "rate_limited", "model provider rate limited the request", True)
                await asyncio.sleep(2**attempt)
            except openai.APIConnectionError:
                if attempt + 1 >= config.max_attempts:
                    return error_response(503, "provider_unavailable", "model provider is unavailable", True)
                await asyncio.sleep(2**attempt)
            except openai.APIStatusError as exc:
                temporary = exc.status_code == 429 or exc.status_code >= 500
                if temporary and attempt + 1 < config.max_attempts:
                    await asyncio.sleep(2**attempt)
                    continue
                status = 503 if temporary else 502
                return error_response(status, "provider_http_error", f"model provider returned HTTP {exc.status_code}", temporary)
        return error_response(502, "semantic_kernel_error", "Semantic Kernel request failed", False)
    finally:
        await client.close()


@app.exception_handler(Exception)
async def unexpected_error(_: Request, __: Exception) -> JSONResponse:
    return error_response(500, "internal_error", "AI kernel internal error", False)
