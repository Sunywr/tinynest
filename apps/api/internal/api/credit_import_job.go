package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"
)

type creditImportJob struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
	Error  string `json:"error,omitempty"`
}

type creditMailProgressKey struct{}

type creditImportInput struct {
	Kind           string `json:"kind"`
	From           string `json:"from"`
	To             string `json:"to"`
	SynchronizedAt string `json:"synchronized_at"`
	Summaries      bool   `json:"summaries"`
	Rows           []struct {
		Key         string  `json:"key"`
		Transaction txInput `json:"transaction"`
	} `json:"rows"`
	Workbook json.RawMessage `json:"workbook"`
}

func (a *API) creditImportJob(w http.ResponseWriter, r *http.Request) {
	p, _ := principal(r)
	card := r.PathValue("id")
	key := [2]string{p.HouseholdID, card}
	if r.Method == http.MethodGet {
		a.creditJobsMu.Lock()
		job := a.creditJobs[key]
		a.creditJobsMu.Unlock()
		writeJSON(w, 200, job)
		return
	}
	var input creditImportInput
	if decode(r, &input) != nil || len(input.Rows) > 10000 ||
		(input.Kind != "sync" && input.Kind != "workbook" && input.Kind != "post") ||
		(input.Kind == "post" && (input.SynchronizedAt == "" || len(input.Rows) == 0 && !input.Summaries)) {
		writeErr(w, 400, "invalid_credit_job", "账单任务参数无效")
		return
	}
	var id string
	if err := a.DB.QueryRowContext(r.Context(), `SELECT id FROM credit_instruments WHERE id=? AND household_id=? AND kind='credit_card' AND status='active'`, card, p.HouseholdID).Scan(&id); err != nil {
		writeErr(w, 404, "missing_card", "信用卡不存在")
		return
	}
	total := len(input.Rows)
	if input.Summaries {
		total++
	}
	a.creditJobsMu.Lock()
	if job := a.creditJobs[key]; job.Status == "running" {
		a.creditJobsMu.Unlock()
		writeErr(w, 409, "credit_job_running", "这张卡已有账单任务在运行，请等待后再提交")
		return
	}
	job := creditImportJob{ID: newID("cjob"), Kind: input.Kind, Status: "running", Total: total}
	if a.creditJobs == nil {
		a.creditJobs = make(map[[2]string]creditImportJob)
	}
	a.creditJobs[key] = job
	a.creditJobsMu.Unlock()
	detached := r.Clone(context.WithoutCancel(r.Context()))
	go a.runCreditImportJob(detached, key, job, input)
	writeJSON(w, 202, job)
}

func (a *API) runCreditImportJob(r *http.Request, key [2]string, job creditImportJob, input creditImportInput) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	publish := func() {
		a.creditJobsMu.Lock()
		a.creditJobs[key] = job
		a.creditJobsMu.Unlock()
	}
	defer func() {
		if recover() != nil {
			job.Status = "failed"
			job.Error = "账单任务中断，请重试"
		}
		publish()
		time.AfterFunc(24*time.Hour, func() {
			a.creditJobsMu.Lock()
			defer a.creditJobsMu.Unlock()
			if a.creditJobs[key].ID == job.ID {
				delete(a.creditJobs, key)
			}
		})
	}()
	call := func(handler http.HandlerFunc, body []byte, pathID, idem string) error {
		req := r.Clone(ctx)
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.SetPathValue("id", pathID)
		req.Header.Set("Idempotency-Key", idem)
		out := httptest.NewRecorder()
		handler(out, req)
		if out.Code >= 200 && out.Code < 300 {
			return nil
		}
		var result struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(out.Body.Bytes(), &result)
		if result.Error.Message == "" {
			result.Error.Message = "账单操作失败"
		}
		return errors.New(result.Error.Message)
	}
	var err error
	switch input.Kind {
	case "sync":
		ctx = context.WithValue(ctx, creditMailProgressKey{}, func(done, total int) {
			job.Done, job.Total = done, total
			publish()
		})
		r.URL.RawQuery = url.Values{"from": {input.From}, "to": {input.To}}.Encode()
		err = call(a.previewEverbrightMail, nil, key[1], "")
	case "workbook":
		err = call(a.importCreditWorkbook, input.Workbook, key[1], "")
	case "post":
		var preview savedCreditPreview
		preview, err = readSavedCreditPreview(ctx, a.DB, key[0], key[1])
		if err == nil && preview.SynchronizedAt != input.SynchronizedAt {
			err = errors.New("预览已更新，请重新打开后入账")
		}
		for _, row := range input.Rows {
			if err != nil {
				break
			}
			row.Transaction.Type = "expense"
			row.Transaction.PaymentKind = "credit"
			cardID := key[1]
			row.Transaction.CreditInstrumentID = &cardID
			if row.Key == "" {
				err = errors.New("流水缺少来源编号")
				break
			}
			var body []byte
			body, err = json.Marshal(row.Transaction)
			if err == nil {
				err = call(a.handleCreateTransaction, body, "", row.Key)
			}
			if err == nil {
				job.Done++
				publish()
			}
		}
		if err == nil && input.Summaries {
			body, _ := json.Marshal(map[string]string{"synchronized_at": input.SynchronizedAt, "from": input.From, "to": input.To})
			err = call(a.postCreditMailSummaries, body, key[1], "")
			if err == nil {
				job.Done++
			}
		}
	}
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		return
	}
	job.Status = "completed"
}
