package dashboard

import (
	"context"
	"net/http"
	"time"
)

func (h *Handler) handleUsageActivity(w http.ResponseWriter, r *http.Request) {
	setCors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	query := r.URL.Query()
	from, through := query.Get("from"), query.Get("through")
	start, startErr := time.Parse("2006-01-02", from)
	end, endErr := time.Parse("2006-01-02", through)
	now := time.Now()
	if len(query["from"]) != 1 || len(query["through"]) != 1 || startErr != nil || endErr != nil ||
		start.Format("2006-01-02") != from || end.Format("2006-01-02") != through ||
		end.Before(start) || end.Sub(start) > 30*24*time.Hour || through > now.Format("2006-01-02") {
		writeError(w, http.StatusBadRequest, "请提供有效的 from / through 日期，最多 31 天且不得晚于今天")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "小时记录暂不可用")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	days, err := h.store.GetUsageActivity(ctx, from, through, now)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "小时记录暂不可用，请稍后重试")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"from": from, "through": through, "today": now.Format("2006-01-02"),
		"timezone": now.Format("MST -07:00"), "generated_at": now.Format(time.RFC3339),
		"source": "request_logs", "days": days,
	})
}
