# LiteLLM Connection Guide

## Configuration

- **LiteLLM Key**: `replace-with-your-litellm-key`
- **LiteLLM API Host**: `http://owlbear:4000/v1`
- **LiteLLM Model Name**: `bulk-gemini25flt-c1`
- **Model**: `gemini/gemini-2.5-flash-lite`
- **Max Tokens**: 65536

## Python Example (OpenAI Compatible)

```python
import openai
client = openai.OpenAI(
    api_key="your_api_key",
    base_url="<your_proxy_base_url>" # LiteLLM Proxy is OpenAI compatible, Read More: https://docs.litellm.ai/docs/proxy/user_keys
)

response = client.chat.completions.create(
    model="gpt-3.5-turbo", # model to send to the proxy
    messages = [
        {
            "role": "user",
            "content": "this is a test request, write a short poem"
        }
    ]
)

print(response)
```

## Model Information

```json
{
  "model_name": "bulk-gemini25flt-c1",
  "litellm_params": {
    "use_in_pass_through": false,
    "use_litellm_proxy": false,
    "merge_reasoning_content_in_choices": false,
    "model": "gemini/gemini-2.5-flash-lite",
    "max_tokens": 65536
  },
  "model_info": {
    "id": "c83a9fbbb0ed2d61c317aaf9fd56af0dd48eb407db783fa79780373c2cacd7c0",
    "db_model": false,
    "supports_vision": true,
    "supports_function_calling": true,
    "supports_reasoning": true,
    "access_via_team_ids": [],
    "direct_access": true,
    "key": "gemini/gemini-2.5-flash-lite",
    "max_tokens": 65535,
    "max_input_tokens": 1048576,
    "max_output_tokens": 65535,
    "input_cost_per_token": 1e-7,
    "input_cost_per_token_flex": null,
    "input_cost_per_token_priority": null,
    "cache_creation_input_token_cost": null,
    "cache_creation_input_token_cost_above_200k_tokens": null,
    "cache_read_input_token_cost": 1e-8,
    "cache_read_input_token_cost_above_200k_tokens": null,
    "cache_read_input_token_cost_above_272k_tokens": null,
    "cache_read_input_token_cost_flex": null,
    "cache_read_input_token_cost_priority": null,
    "cache_creation_input_token_cost_above_1hr": null,
    "input_cost_per_character": null,
    "input_cost_per_token_above_128k_tokens": null,
    "input_cost_per_token_above_200k_tokens": null,
    "input_cost_per_token_above_272k_tokens": null,
    "input_cost_per_query": null,
    "input_cost_per_second": null,
    "input_cost_per_audio_token": 3e-7,
    "input_cost_per_image_token": null,
    "input_cost_per_image": null,
    "input_cost_per_audio_per_second": null,
    "input_cost_per_video_per_second": null,
    "input_cost_per_token_batches": null,
    "output_cost_per_token_batches": null,
    "output_cost_per_token": 4e-7,
    "output_cost_per_token_flex": null,
    "output_cost_per_token_priority": null,
    "regional_processing_uplift_multiplier_eu": null,
    "regional_processing_uplift_multiplier_us": null,
    "output_cost_per_audio_token": null,
    "output_cost_per_character": null,
    "output_cost_per_reasoning_token": 4e-7,
    "output_cost_per_token_above_128k_tokens": null,
    "output_cost_per_character_above_128k_tokens": null,
    "output_cost_per_token_above_200k_tokens": null,
    "output_cost_per_token_above_272k_tokens": null,
    "output_cost_per_second": null,
    "output_cost_per_second_1080p": null,
    "output_cost_per_video_per_second": null,
    "output_cost_per_image": null,
    "output_cost_per_image_token": null,
    "output_vector_size": null,
    "citation_cost_per_token": null,
    "tiered_pricing": null,
    "litellm_provider": "gemini",
    "mode": "chat",
    "supports_system_messages": true,
    "supports_response_schema": true,
    "supports_tool_choice": true,
    "supports_assistant_prefill": null,
    "supports_prompt_caching": true,
    "supports_audio_input": null,
    "supports_audio_output": false,
    "supports_pdf_input": true,
    "supports_embedding_image_input": null,
    "supports_native_streaming": null,
    "supports_native_structured_output": null,
    "supports_web_search": true,
    "supports_url_context": true,
    "supports_none_reasoning_effort": null,
    "supports_minimal_reasoning_effort": null,
    "supports_low_reasoning_effort": null,
    "supports_xhigh_reasoning_effort": null,
    "supports_max_reasoning_effort": null,
    "bedrock_output_config_effort_ceiling": null,
    "supports_computer_use": null,
    "search_context_cost_per_query": {
      "search_context_size_low": 0.035,
      "search_context_size_medium": 0.035,
      "search_context_size_high": 0.035
    },
    "tpm": 250000,
    "rpm": 15,
    "ocr_cost_per_page": null,
    "ocr_cost_per_credit": null,
    "annotation_cost_per_page": null,
    "provider_specific_entry": null,
    "uses_embed_content": null,
    "supports_image_size": false,
    "supported_openai_params": [
      "temperature",
      "top_p",
      "max_tokens",
      "max_completion_tokens",
      "stream",
      "tools",
      "tool_choice",
      "functions",
      "response_format",
      "n",
      "stop",
      "logprobs",
      "frequency_penalty",
      "presence_penalty",
      "modalities",
      "parallel_tool_calls",
      "web_search_options",
      "include_server_side_tool_invocations",
      "service_tier",
      "reasoning_effort",
      "thinking"
    ]
  },
  "provider": "gemini",
  "input_cost": "0.10",
  "output_cost": "0.40",
  "litellm_model_name": "gemini/gemini-2.5-flash-lite",
  "max_tokens": 65535,
  "max_input_tokens": 1048576,
  "cleanedLitellmParams": {
    "use_in_pass_through": false,
    "use_litellm_proxy": false,
    "merge_reasoning_content_in_choices": false,
    "max_tokens": 65536
  }
}
```

## Note for Go Lang Projects

To use this configuration in a Go lang project, you can leverage an OpenAI-compatible client library. The LiteLLM proxy exposes an OpenAI-compatible API, so any OpenAI client will work.

### Example using [`go-openai`](https://github.com/sashabaranov/go-openai):

```go
package main

import (
    "context"
    "fmt"
    "log"

    openai "github.com/sashabaranov/go-openai"
)

func main() {
    // Initialize client with your LiteLLM key
    client := openai.NewClient("your_api_key")
    
    // Set the base URL to your LiteLLM proxy
    client.BaseURL = "<your_proxy_base_url>"

    resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
        Model: "gpt-3.5-turbo", // model to send to the proxy
        Messages: []openai.ChatCompletionMessage{
            {
                Role:    openai.ChatMessageRoleUser,
                Content: "this is a test request, write a short poem",
            },
        },
    })
    if err != nil {
        log.Fatalf("ChatCompletion error: %v", err)
    }

    fmt.Println(resp.Choices[0].Message.Content)
}
```

### Important Notes:
1. Replace `"your_api_key"` with your actual LiteLLM key (`replace-with-your-litellm-key`)
2. Replace `"<your_proxy_base_url>"` with your LiteLLM proxy URL (`http://owlbear:4000/v1`)
3. The model parameter in the request (`"gpt-3.5-turbo"` in the example) is the identifier sent to the proxy - your proxy will map this to the actual model (`bulk-gemini25flt-c1` → `gemini/gemini-2.5-flash-lite`)
4. All OpenAI-compatible parameters (temperature, top_p, max_tokens, etc.) are supported as shown in the model info's `supported_openai_params` array

## Key Features of This Model
- **Vision Support**: ✅ (supports image inputs)
- **Function Calling**: ✅
- **Reasoning**: ✅
- **PDF Input**: ✅
- **Web Search**: ✅
- **Prompt Caching**: ✅
- **Token Limits**: 1M input tokens, 65K output tokens
- **Cost**: $0.10 per 1M input tokens, $0.40 per 1M output tokens

---

*This guide was generated from your LiteLLM configuration. For more details, refer to the [LiteLLM documentation](https://docs.litellm.ai).*
