package telemetry

// The names of the OpenTelemetry semantic conventions for generative AI that
// kubedojo records, in one place.
//
// Matched against: github.com/open-telemetry/semantic-conventions-genai,
// tag v0.1.0 (commit d0e74ba6c48bcc4ded473b1828a8212f06b23753, 2026-11-21),
// which builds on semantic-conventions v1.45.0:
//
//	docs/gen-ai/client-inference.md      the span, the duration metrics
//	docs/gen-ai/gen-ai-token-metrics.md  the token metrics
//	docs/gen-ai/gen-ai-spans.md          how content is recorded
//	docs/gen-ai/anthropic.md, openai.md  what a provider's token counts mean
//	model/gen-ai/gen-ai-*-messages.json  the shape of recorded messages
//
// The conventions have the status "Development": names can change. v0.1.0 is
// the first release since they left open-telemetry/semantic-conventions
// (last there: v1.41.0), and it renamed the client metrics. A later change is
// an edit of this file, of the dashboard and of docs/observability.md.
//
// No semconv package is imported for these on purpose: the Go packages
// follow the conventions with a delay, and a constant here is easier to
// compare with the text of the conventions than a generated identifier.

// The span of one call to a model ("gen_ai.client.inference"). Its name is
// "{gen_ai.operation.name} {gen_ai.request.model}", its kind CLIENT.
const (
	genaiOperationChat = "chat"

	// Required, and set when the span starts: a sampler may read them.
	genaiOperationName = "gen_ai.operation.name"
	genaiProviderName  = "gen_ai.provider.name"
	genaiRequestModel  = "gen_ai.request.model"
	serverAddress      = "server.address"
	serverPort         = "server.port"

	// The request.
	genaiConversationID   = "gen_ai.conversation.id"
	genaiRequestStream    = "gen_ai.request.stream" // only if the request streams
	genaiRequestMaxTokens = "gen_ai.request.max_tokens"

	// The response.
	genaiResponseID            = "gen_ai.response.id"
	genaiResponseModel         = "gen_ai.response.model"
	genaiResponseFinishReasons = "gen_ai.response.finish_reasons" // string[]
	// Seconds from the start of the call to the first chunk of the stream.
	genaiResponseTimeToFirstChunk = "gen_ai.response.time_to_first_chunk"

	// Token counts. input_tokens includes the tokens read from and written
	// to a prompt cache; the two cache attributes are subsets of it and are
	// set only when they are not zero.
	genaiUsageInputTokens      = "gen_ai.usage.input_tokens"
	genaiUsageOutputTokens     = "gen_ai.usage.output_tokens"
	genaiUsageCacheReadTokens  = "gen_ai.usage.cache_read.input_tokens"
	genaiUsageCacheWriteTokens = "gen_ai.usage.cache_write.input_tokens"

	// Set if, and only if, the call ended in an error.
	errorType = "error.type"

	// Content: opt-in, and recorded as structured values (AnyValue), not as
	// JSON text.
	//
	//	gen_ai.system_instructions  [{type: "text", content}]
	//	gen_ai.input.messages       [{role, parts: [{type: "text", content}]}]
	//	gen_ai.output.messages      [{role: "assistant", parts: [{type: "text", content}]}]
	genaiSystemInstructions = "gen_ai.system_instructions"
	genaiInputMessages      = "gen_ai.input.messages"
	genaiOutputMessages     = "gen_ai.output.messages"
	genaiMessageRole        = "role"
	genaiMessageParts       = "parts"
	genaiPartType           = "type"
	genaiPartTypeText       = "text"
	genaiPartContent        = "content"
)

// Finish reasons with a fixed meaning. A reason the provider reports is
// recorded as it is.
const (
	// The generation ended abnormally: it failed, was cancelled, or the
	// stream ended before its last event.
	genaiFinishError = "error"
	// The model declined to answer.
	genaiFinishContentFilter = "content_filter"
)

// The client metrics. The histograms carry genaiOperationName,
// genaiProviderName, genaiRequestModel, genaiResponseModel, serverAddress
// and serverPort; the duration and the per-operation token histograms carry
// errorType in addition, the usage counters genaiTokenModality.
const (
	genaiMetricDuration         = "gen_ai.client.inference.duration"            // s
	genaiMetricTimeToFirstChunk = "gen_ai.client.inference.time_to_first_chunk" // s

	// Counters, for totals and rates. Only positive counts are added.
	genaiMetricInputTokens      = "gen_ai.client.inference.usage.input_tokens"
	genaiMetricOutputTokens     = "gen_ai.client.inference.usage.output_tokens"
	genaiMetricCacheReadTokens  = "gen_ai.client.inference.usage.cache_read.input_tokens"
	genaiMetricCacheWriteTokens = "gen_ai.client.inference.usage.cache_write.input_tokens"

	// Histograms, for the distribution over calls. Every call is recorded,
	// with 0 when the provider reported no usage.
	genaiMetricOperationInputTokens  = "gen_ai.client.inference.operation.input_tokens"
	genaiMetricOperationOutputTokens = "gen_ai.client.inference.operation.output_tokens"

	genaiUnitToken = "{token}"

	genaiTokenModality     = "gen_ai.token.modality"
	genaiTokenModalityText = "text"
)

// The bucket boundaries the conventions give.
var (
	genaiDurationBuckets = []float64{0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28, 2.56, 5.12, 10.24, 20.48, 40.96, 81.92}
	genaiTokenBuckets    = []float64{1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864}
)
