import importlib
import json
import asyncio
import base64
import os
from types import SimpleNamespace

import httpx
from starlette.testclient import TestClient


def load_app():
    os.environ["AI_KERNEL_INTERNAL_TOKEN"] = "test-token"
    module = importlib.import_module("app")
    return TestClient(module.app)


def test_healthz():
    response = load_app().get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok", "version": 2}


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
            assert request["max_tokens"] == 8192
            return SimpleNamespace(
                choices=[
                    SimpleNamespace(
                        message=SimpleNamespace(content='{"ok":true}'),
                        finish_reason="stop",
                    )
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
                response_format={"type": "json_object"},
                max_tokens=8192,
            ),
        )
        assert message is not None
        assert message.content == '{"ok":true}'
        assert message.metadata["usage"].prompt_tokens == 2
        assert message.metadata["finish_reason"] == "stop"

    asyncio.run(invoke())


def test_semantic_kernel_connector_attaches_images_to_first_user_message():
    module = importlib.import_module("app")
    encoded = base64.b64encode(b"jpeg").decode("ascii")

    class FakeCompletions:
        async def create(self, **request):
            assert request["messages"] == [
                {"role": "system", "content": "system"},
                {
                    "role": "user",
                    "content": [
                        {"type": "text", "text": "user"},
                        {
                            "type": "image_url",
                            "image_url": {
                                "url": f"data:image/jpeg;base64,{encoded}",
                                "detail": "high",
                            },
                        },
                    ],
                },
            ]
            return SimpleNamespace(
                choices=[
                    SimpleNamespace(
                        message=SimpleNamespace(content='{"ok":true}'),
                        finish_reason="stop",
                    )
                ],
                usage=SimpleNamespace(prompt_tokens=2, completion_tokens=1),
            )

    async def invoke():
        service = module.OpenAICompatibleChatCompletion(
            ai_model_id="vision-model",
            client=SimpleNamespace(
                chat=SimpleNamespace(completions=FakeCompletions())
            ),
        )
        history = module.ChatHistory(system_message="system")
        history.add_user_message("user")
        message = await service.get_chat_message_content(
            history,
            module.OpenAICompatibleExecutionSettings(
                response_format={"type": "json_object"},
                images=[
                    module.ImageInput(
                        media_type="image/jpeg",
                        data_base64=encoded,
                    )
                ],
            ),
        )
        assert message is not None
        assert message.content == '{"ok":true}'

    asyncio.run(invoke())


def test_generate_json_object_supplies_schema_and_targeted_repair(monkeypatch):
    os.environ["AI_KERNEL_INTERNAL_TOKEN"] = "test-token"
    module = importlib.import_module("app")
    calls = []
    outputs = iter(['{"wrong":true}', '{"ok":true}'])

    class FakeCompletions:
        async def create(self, **request):
            calls.append(request)
            return SimpleNamespace(
                choices=[
                    SimpleNamespace(
                        message=SimpleNamespace(content=next(outputs)),
                        finish_reason="stop",
                    )
                ],
                usage=SimpleNamespace(prompt_tokens=4, completion_tokens=2),
            )

    class FakeClient:
        def __init__(self):
            self.chat = SimpleNamespace(completions=FakeCompletions())

        async def close(self):
            pass

    monkeypatch.setattr(module, "AsyncOpenAI", lambda **_: FakeClient())
    schema = {
        "type": "object",
        "additionalProperties": False,
        "required": ["ok"],
        "properties": {"ok": {"const": True}},
    }
    response = TestClient(module.app).post(
        "/v1/generate",
        headers={"X-Anby-Internal-Token": "test-token"},
        json={
            "version": 1,
            "config": {
                "provider": "deepseek",
                "base_url": "https://api.deepseek.com",
                "api_key": "not-used",
                "model": "deepseek-v4-flash",
                "response_format": "json_object",
                "request_timeout_seconds": 30,
                "max_attempts": 2,
            },
            "system_prompt": "Return JSON.",
            "user_prompt": "Return the requested object.",
            "json_schema": schema,
        },
    )

    assert response.status_code == 200
    assert response.json()["json"] == {"ok": True}
    assert len(calls) == 2
    assert response.json()["input_tokens"] == 8
    assert response.json()["output_tokens"] == 4
    assert [a["status"] for a in response.json()["attempts"]] == ["invalid_output", "succeeded"]
    assert "Required output JSON Schema" in calls[0]["messages"][0]["content"]
    assert '"required":["ok"]' in calls[0]["messages"][0]["content"]
    assert calls[1]["messages"][-2] == {"role": "assistant", "content": '{"wrong":true}'}
    assert "at $ (additionalProperties)" in calls[1]["messages"][-1]["content"]


def test_generate_reports_output_truncation(monkeypatch):
    os.environ["AI_KERNEL_INTERNAL_TOKEN"] = "test-token"
    module = importlib.import_module("app")

    class FakeCompletions:
        async def create(self, **_):
            return SimpleNamespace(
                choices=[
                    SimpleNamespace(
                        message=SimpleNamespace(content='{"partial":true}'),
                        finish_reason="length",
                    )
                ],
                usage=SimpleNamespace(prompt_tokens=4, completion_tokens=8192),
            )

    class FakeClient:
        def __init__(self):
            self.chat = SimpleNamespace(completions=FakeCompletions())

        async def close(self):
            pass

    monkeypatch.setattr(module, "AsyncOpenAI", lambda **_: FakeClient())
    response = TestClient(module.app).post(
        "/v1/generate",
        headers={"X-Anby-Internal-Token": "test-token"},
        json={
            "version": 1,
            "config": {
                "provider": "deepseek",
                "base_url": "https://api.deepseek.com",
                "api_key": "not-used",
                "model": "deepseek-v4-flash",
                "response_format": "json_object",
                "request_timeout_seconds": 30,
                "max_attempts": 1,
            },
            "system_prompt": "Return JSON.",
            "user_prompt": "Return the requested object.",
            "json_schema": {"type": "object"},
        },
    )

    assert response.status_code == 502
    assert response.json()["code"] == "output_truncated"
    assert response.json()["attempts"][0]["output_tokens"] == 8192
    assert response.json()["attempts"][0]["usage_reported"] is True


def test_schema_validation_location_excludes_model_values():
    module = importlib.import_module("app")
    schema = {
        "type": "object",
        "required": ["secret"],
        "properties": {"secret": {"type": "integer"}},
    }
    try:
        module.jsonschema.validate(
            instance={"secret": "sensitive model output"},
            schema=schema,
        )
    except module.jsonschema.ValidationError as error:
        location = module.schema_validation_location(error)
    else:
        raise AssertionError("schema validation unexpectedly passed")

    assert location == "$.secret (type)"
    assert "sensitive model output" not in location


def test_generate_rejects_invalid_image_base64():
    os.environ["AI_KERNEL_INTERNAL_TOKEN"] = "test-token"
    module = importlib.import_module("app")
    response = TestClient(module.app).post(
        "/v1/generate",
        headers={"X-Anby-Internal-Token": "test-token"},
        json={
            "version": 1,
            "config": {
                "provider": "deepseek",
                "base_url": "https://api.deepseek.com",
                "api_key": "not-used",
                "model": "deepseek-flash",
                "response_format": "json_object",
                "request_timeout_seconds": 30,
                "max_attempts": 1,
            },
            "system_prompt": "Return JSON.",
            "user_prompt": "Analyze the image.",
            "json_schema": {"type": "object"},
            "images": [
                {
                    "media_type": "image/jpeg",
                    "data_base64": "not-base64!",
                }
            ],
        },
    )
    assert response.status_code == 422
    assert response.json()["code"] == "invalid_image"


def test_provider_status_error_code_is_safe_and_actionable():
    module = importlib.import_module("app")

    assert module.provider_status_error_code(401) == "authentication_failed"
    assert module.provider_status_error_code(403) == "authentication_failed"
    assert module.provider_status_error_code(429) == "rate_limited"
    assert module.provider_status_error_code(400) == "invalid_request"
    assert module.provider_status_error_code(404) == "invalid_request"
    assert module.provider_status_error_code(500) == "provider_unavailable"


def test_quota_exhaustion_does_not_retry_or_expose_supplier_message(monkeypatch):
    module = importlib.import_module("app")
    calls = []

    class FakeCompletions:
        async def create(self, **request):
            calls.append(request)
            raise module.openai.RateLimitError(
                "private supplier message",
                response=httpx.Response(429, request=httpx.Request("POST", "https://provider.invalid")),
                body={"code": "insufficient_quota"},
            )

    class FakeClient:
        def __init__(self):
            self.chat = SimpleNamespace(completions=FakeCompletions())

        async def close(self):
            pass

    monkeypatch.setattr(module, "AsyncOpenAI", lambda **_: FakeClient())
    response = load_app().post(
        "/v1/generate",
        headers={"X-Anby-Internal-Token": "test-token"},
        json={
            "version": 1,
            "config": {
                "provider": "openai-compatible", "base_url": "https://provider.invalid",
                "api_key": "not-used", "model": "test-model", "response_format": "json_schema",
                "request_timeout_seconds": 30, "max_attempts": 3,
            },
            "system_prompt": "Return JSON.", "user_prompt": "Return an empty object.",
            "json_schema": {"type": "object"},
        },
    )
    assert response.status_code == 429
    assert response.json()["code"] == module.QUOTA_EXHAUSTED_CODE
    assert response.json()["temporary"] is False
    assert len(calls) == 1
    assert "private supplier" not in response.text
    assert response.json()["attempts"][0]["usage_reported"] is False


def test_v2_stream_receipts_include_failed_attempts_and_request_ids(monkeypatch):
    module = importlib.import_module("app")
    requests = []
    outputs = iter(['{"wrong":true}', '{"ok":true}'])

    class FakeCompletions:
        async def create(self, **request):
            requests.append(request)
            return SimpleNamespace(
                choices=[SimpleNamespace(message=SimpleNamespace(content=next(outputs)), finish_reason="stop")],
                usage=SimpleNamespace(prompt_tokens=7, completion_tokens=3),
            )

    class FakeClient:
        def __init__(self):
            self.chat = SimpleNamespace(completions=FakeCompletions())

        async def close(self):
            pass

    monkeypatch.setattr(module, "AsyncOpenAI", lambda **_: FakeClient())
    response = load_app().post("/v1/generate", headers={"X-Anby-Internal-Token": "test-token"}, json={
        "version": 2, "config": {
            "provider": "deepseek", "base_url": "https://provider.invalid", "api_key": "not-used",
            "model": "test", "response_format": "json_object", "request_timeout_seconds": 30, "max_attempts": 2,
        }, "system_prompt": "Return JSON.", "user_prompt": "Return ok.",
        "json_schema": {"type": "object", "required": ["ok"], "properties": {"ok": {"const": True}}},
    })
    events = [json.loads(line) for line in response.text.splitlines()]
    from pathlib import Path
    import jsonschema
    schema = json.loads(Path(__file__).with_name("generate-stream-v2.schema.json").read_text())
    for event in events:
        jsonschema.validate(event, schema, format_checker=jsonschema.FormatChecker())
    assert [e["type"] for e in events] == [
        "attempt_started", "attempt_finished", "attempt_started", "attempt_finished", "result",
    ]
    for index in range(2):
        assert requests[index]["extra_headers"]["X-Client-Request-Id"] == events[index*2]["attempt"]["id"]
    assert events[1]["attempt"]["status"] == "invalid_output"
    assert events[1]["attempt"]["input_tokens"] == 7
    assert events[-1]["body"]["input_tokens"] == 14
    assert events[-1]["status"] == 200
