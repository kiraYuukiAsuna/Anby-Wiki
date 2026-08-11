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
    assert "Required output JSON Schema" in calls[0]["messages"][0]["content"]
    assert '"required":["ok"]' in calls[0]["messages"][0]["content"]
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
