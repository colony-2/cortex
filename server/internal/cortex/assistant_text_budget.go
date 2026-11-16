package cortex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	ca "github.com/colony-2/cortex-assistant/assistant"
)

// Only effective text allowances cross the browser boundary. Bindings and the
// remaining execution policy stay in the facade.
type AssistantTextBudget struct {
	RawUTF8Bytes             int `json:"raw_utf8_bytes"`
	GoJSONStringContentBytes int `json:"go_json_string_content_bytes,omitempty"`
	HTTPBodyBytes            int `json:"http_body_bytes"`
}

type assistantDraftRejection struct {
	message string
	cause   error
}

func (e *assistantDraftRejection) Error() string { return e.message }
func (e *assistantDraftRejection) Unwrap() error { return e.cause }
func rejectAssistantDraft(message string, cause error) error {
	return &assistantDraftRejection{message + ". Your complete draft is still editable.", cause}
}

func assistantTextBudget(limits ca.Limits, protocol string) *AssistantTextBudget {
	if limits.Human <= 0 {
		return nil
	}
	switch protocol {
	case ca.GoalProtocol, ca.OrdinaryProtocol, ca.Protocol, "cortex-assistant/1":
	default:
		return nil
	}
	b := &AssistantTextBudget{RawUTF8Bytes: min(limits.Human, 32<<10), HTTPBodyBytes: assistantBodyLimit}
	if protocol == ca.GoalProtocol {
		b.GoJSONStringContentBytes = limits.Human
	}
	return b
}
func assistantSessionBudget(s ca.Session) *AssistantTextBudget {
	if s.Current == nil || s.Current.Form.Request == nil {
		return nil
	}
	raw, err := json.Marshal(*s.Current.Form.Request)
	if err != nil {
		return nil
	}
	var request struct {
		Protocol string `json:"protocol"`
	}
	if json.Unmarshal(raw, &request) != nil {
		return nil
	}
	return assistantTextBudget(s.Limits, request.Protocol)
}
func validateAssistantText(text string, budget *AssistantTextBudget) error {
	if budget == nil {
		return ErrAssistantUnavailable
	}
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
		return rejectAssistantDraft("Use a nonblank UTF-8 message", ErrAssistantInvalid)
	}
	if len(text) > budget.RawUTF8Bytes {
		return rejectAssistantDraft(fmt.Sprintf("This message exceeds %d UTF-8 bytes. Shorten it", budget.RawUTF8Bytes), ErrAssistantInvalid)
	}
	encoded, _ := json.Marshal(text)
	if budget.GoJSONStringContentBytes > 0 && len(encoded)-2 > budget.GoJSONStringContentBytes {
		return rejectAssistantDraft(fmt.Sprintf("This message exceeds %d bytes after service JSON encoding. Shorten it", budget.GoJSONStringContentBytes), ErrAssistantInvalid)
	}
	return nil
}

// Only parsing and preparation paths that have never invoked submission use this.
func writeAssistantDraftRejection(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error      string `json:"error"`
		Code       string `json:"code"`
		Submission string `json:"submission"`
	}{message, "draft_rejected", "not_started"})
}
