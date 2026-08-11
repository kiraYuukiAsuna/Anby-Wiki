import importlib
import asyncio
import os
from types import SimpleNamespace

from fastapi.testclient import TestClient


def load_app():
    os.environ["AI_KERNEL_INTERNAL_TOKEN"] = "test-token"
    module = importlib.import_module("app")
    return TestClient(module.app)


def test_healthz():
    response = load_app().get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok", "version": 1}


def test_generate_rejects_missing_internal_token():
    response = load_app().post(
        "/v1/generate",
        json={
            "version": 1,
            "config": {
                "provider": "deepseek",
                "base_url": "https://api.deepseek.com",
                "api_key": "not-used",
                "model": "deepseek-chat",
                "response_format": "json_object",
                "request_timeout_seconds": 30,
                "max_attempts": 1,
            },
            "system_prompt": "Return JSON.",
            "user_prompt": "Return an empty object.",
            "json_schema": {"type": "object"},
        },
    )
    assert response.status_code == 401
    assert response.json()["code"] == "unauthorized"


def test_semantic_kernel_connector_maps_chat_and_usage():
    module = importlib.import_module("app")

    class FakeCompletions:
        async def create(self, **request):
            assert request["messages"] == [
                {"role": "system", "content": "system"},
                {"role": "user", "content": "user"},
            ]
            assert request["response_format"] == {"type": "json_object"}
            return SimpleNamespace(
                choices=[
                    SimpleNamespace(message=SimpleNamespace(content='{"ok":true}'))
                ],
                usage=SimpleNamespace(prompt_tokens=2, completion_tokens=1),
            )

    async def invoke():
        service = module.OpenAICompatibleChatCompletion(
            ai_model_id="test-model",
            client=SimpleNamespace(
                chat=SimpleNamespace(completions=FakeCompletions())
            ),
        )
        history = module.ChatHistory(system_message="system")
        history.add_user_message("user")
        message = await service.get_chat_message_content(
            history,
            module.OpenAICompatibleExecutionSettings(
                response_format={"type": "json_object"}
            ),
        )
        assert message is not None
        assert message.content == '{"ok":true}'
        assert message.metadata["usage"].prompt_tokens == 2

    asyncio.run(invoke())
