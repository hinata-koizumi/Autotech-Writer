"""LLM provider implementations."""

from abc import ABC, abstractmethod
from typing import Optional, Dict

from openai import AsyncOpenAI
from anthropic import AsyncAnthropic

from app.config import Config


class LLMProvider(ABC):
    """Abstract base class for LLM providers."""

    @abstractmethod
    async def call(
        self, 
        system_prompt: str, 
        user_prompt: str, 
        temperature: float = 0.5, 
        max_tokens: int = 4000,
        is_triage: bool = False
    ) -> str:
        """Send prompt and return raw response text."""
        pass


class OpenAIProvider(LLMProvider):
    def __init__(self, api_key: str, model_triage: str, model_gen: str):
        self.client = AsyncOpenAI(api_key=api_key)
        self.model_triage = model_triage
        self.model_gen = model_gen

    async def call(
        self, 
        system_prompt: str, 
        user_prompt: str, 
        temperature: float = 0.5, 
        max_tokens: int = 4000,
        is_triage: bool = False
    ) -> str:
        model = self.model_triage if is_triage else self.model_gen
        messages = []
        if system_prompt:
            messages.append({"role": "system", "content": system_prompt})
        messages.append({"role": "user", "content": user_prompt})

        response = await self.client.chat.completions.create(
            model=model,
            messages=messages,
            temperature=temperature,
            max_tokens=max_tokens,
        )
        return response.choices[0].message.content or ""


class AnthropicProvider(LLMProvider):
    def __init__(self, api_key: str, model_triage: str, model_gen: str):
        self.client = AsyncAnthropic(api_key=api_key)
        self.model_triage = model_triage
        self.model_gen = model_gen

    async def call(
        self, 
        system_prompt: str, 
        user_prompt: str, 
        temperature: float = 0.5, 
        max_tokens: int = 4000,
        is_triage: bool = False
    ) -> str:
        model = self.model_triage if is_triage else self.model_gen
        
        response = await self.client.messages.create(
            model=model,
            max_tokens=max_tokens,
            temperature=temperature,
            system=system_prompt if system_prompt else None,
            messages=[{"role": "user", "content": user_prompt}],
        )
        return response.content[0].text


class LLMProviderFactory:
    """Factory to create LLM providers."""

    @staticmethod
    def create_providers(config: Config) -> Dict[str, LLMProvider]:
        providers = {}
        llm_cfg = config.llm
        
        if llm_cfg.openai_api_key:
            providers["openai"] = OpenAIProvider(
                api_key=llm_cfg.openai_api_key,
                model_triage=llm_cfg.model_triage_openai,
                model_gen=llm_cfg.model_gen_openai
            )
        if llm_cfg.anthropic_api_key:
            providers["anthropic"] = AnthropicProvider(
                api_key=llm_cfg.anthropic_api_key,
                model_triage=llm_cfg.model_triage_anthropic,
                model_gen=llm_cfg.model_gen_anthropic
            )
        return providers
