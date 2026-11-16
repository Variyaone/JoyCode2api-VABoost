package anthropic

import "testing"

// Regression for the 2026-11-15 outage: GLM-family upstream replies
// "tools 数量超过上限 128" (invalid_request_error, param=tools) when the
// client mounts more tool schemas than the model's cap. Claude Code sessions
// with 160+ mounted tools died 60 consecutive 500s (request_logs
// 147889-147980) because every identical retry failed the same way.
// The classifier must catch both the Chinese message and the param=tools
// shape, and dropToolSchemas must clear the over-cap payload.
func TestIsToolsLimitError(t *testing.T) {
	// Raw upstream body observed in request_logs id=147980.
	observed := `upstream Chat response error: {"cause":"{\"error\":{\"message\":\"tools 数量超过上限 128\",\"type\":\"invalid_request_error\",\"param\":\"tools\",\"code\":\"invalid_request_error\"}}","code":400,"message":"模型服务调用失败","status":"FAILED_RESPONSE"}`
	if !isToolsLimitError(observed) {
		t.Fatal("observed 2026-11-15 upstream body not classified as tools-limit")
	}
	if !isToolsLimitError(`{"error":{"message":"tools 数量超过上限 128","type":"invalid_request_error","param":"tools"}}`) {
		t.Fatal("bare Chinese message not classified")
	}
	if !isToolsLimitError(`{"error":{"message":"too many tools","type":"invalid_request_error","param":"tools"}}`) {
		t.Fatal("param=tools shape not classified")
	}
	if isToolsLimitError(`{"error":{"message":"tools 数量不能超过 128","type":"invalid_request_error","param":"model"}}`) {
		t.Fatal("param=model must not classify as tools-limit")
	}
	if isToolsLimitError(`{"error":{"message":"invalid tools schema","type":"invalid_request_error"}}`) {
		t.Fatal("generic invalid tools message must not classify")
	}
}

func TestToolsLimitRepairDropsSchemas(t *testing.T) {
	req := &MessageRequest{Tools: make([]Tool, 160)}
	if !isToolsLimitError(`{"error":{"message":"tools 数量超过上限 128","param":"tools"}}`) {
		t.Fatal("precondition: error must classify")
	}
	if !dropToolSchemas(req) {
		t.Fatal("dropToolSchemas must drop when tools present")
	}
	if req.Tools != nil {
		t.Fatal("tools must be nil after drop")
	}
	if dropToolSchemas(req) {
		t.Fatal("second drop must report nothing changed")
	}
}
