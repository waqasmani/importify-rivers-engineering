package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/auth"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/filestore"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/normalize"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/valid"
)

func (s *Server) mountActivity(r chi.Router) {
	r.Route("/tasks", func(r chi.Router) {
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/", s.h(s.handleListTasks))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/reminders", s.h(s.handleTaskReminders))
		r.With(s.requirePerm(auth.PermDataExport)).Get("/export", s.h(s.handleExport("task")))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/", s.h(s.handleCreateTask))
		r.With(s.requirePerm(auth.PermCRMWrite)).Post("/bulk", s.h(s.handleBulk("task", s.taskBulkActions())))
		r.With(s.requirePerm(auth.PermCRMRead)).Get("/{id}", s.h(s.handleGetTask))
		r.With(s.requirePerm(auth.PermCRMWrite)).Put("/{id}", s.h(s.handleUpdateTask))
		r.With(s.requirePerm(auth.PermCRMWrite)).Delete("/{id}", s.h(s.handleDeleteTask))
	})
	r.Get("/notes", s.h(s.handleListNotes))
	r.Post("/notes", s.h(s.handleCreateNote))
	r.Put("/notes/{id}", s.h(s.handleUpdateNote))
	r.Delete("/notes/{id}", s.h(s.handleDeleteNote))

	r.Get("/attachments", s.h(s.handleListAttachments))
	r.Post("/attachments", s.h(s.handleUploadAttachment))
	r.Get("/attachments/{id}/download", s.h(s.handleDownloadAttachment))
	r.Delete("/attachments/{id}", s.h(s.handleDeleteAttachment))

	r.Get("/tags", s.h(s.handleListTags))
	r.With(s.requirePerm(auth.PermCRMWrite, auth.PermProductsWrite)).Post("/tags", s.h(s.handleCreateTag))
	r.With(s.requirePerm(auth.PermSettingsManage)).Put("/tags/{id}", s.h(s.handleUpdateTag))
	r.With(s.requirePerm(auth.PermSettingsManage)).Delete("/tags/{id}", s.h(s.handleDeleteTag))
	r.Put("/record-tags", s.h(s.handleSetRecordTags))

	r.Get("/custom-fields", s.h(s.handleListCustomFields))
	r.With(s.requirePerm(auth.PermSettingsManage)).Post("/custom-fields", s.h(s.handleCreateCustomField))
	r.With(s.requirePerm(auth.PermSettingsManage)).Put("/custom-fields/{id}", s.h(s.handleUpdateCustomField))
	r.With(s.requirePerm(auth.PermSettingsManage)).Delete("/custom-fields/{id}", s.h(s.handleDeleteCustomField))

	r.Get("/saved-filters", s.h(s.handleListSavedFilters))
	r.Post("/saved-filters", s.h(s.handleCreateSavedFilter))
	r.Delete("/saved-filters/{id}", s.h(s.handleDeleteSavedFilter))
}

// ---- tasks ----

var taskStatuses = []string{"open", "in_progress", "done", "cancelled"}
var taskPriorities = []string{"low", "normal", "high", "urgent"}
var taskTypes = []string{"todo", "follow_up", "call", "email", "meeting", "review"}

type taskInput struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	TaskType    string     `json:"task_type"`
	DueAt       *time.Time `json:"due_at"`
	RemindAt    *time.Time `json:"remind_at"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	EntityType  *string    `json:"entity_type"`
	EntityID    *uuid.UUID `json:"entity_id"`
}

func (s *Server) prepareTask(r *http.Request, in *taskInput, existing *store.Task) (store.UpdateTaskParams, error) {
	in.Title = normalize.Text(in.Title)
	in.Description = normalize.MultilineText(in.Description)
	if in.Status == "" {
		in.Status = "open"
	}
	if in.Priority == "" {
		in.Priority = "normal"
	}
	if in.TaskType == "" {
		in.TaskType = "todo"
	}
	if in.EntityType != nil && *in.EntityType == "" {
		in.EntityType = nil
	}
	v := valid.New()
	v.Len("title", in.Title, 1, 200)
	v.Max("description", in.Description, 20000)
	v.OneOf("status", in.Status, taskStatuses...)
	v.OneOf("priority", in.Priority, taskPriorities...)
	v.OneOf("task_type", in.TaskType, taskTypes...)
	v.Check((in.EntityType == nil) == (in.EntityID == nil), "entity_id", "and entity_type must be provided together")
	s.checkUserRef(r.Context(), in.AssigneeID, "assignee_id", v)
	if err := checkValid(v); err != nil {
		return store.UpdateTaskParams{}, err
	}
	if in.EntityType != nil {
		if err := s.requireEntity(r, *in.EntityType, *in.EntityID, false); err != nil {
			return store.UpdateTaskParams{}, err
		}
	}
	var completed *time.Time
	if in.Status == "done" {
		now := time.Now()
		completed = &now
		if existing != nil && existing.CompletedAt != nil {
			completed = existing.CompletedAt
		}
	}
	return store.UpdateTaskParams{Title: in.Title, Description: in.Description, Status: in.Status, Priority: in.Priority,
		TaskType: in.TaskType, DueAt: in.DueAt, RemindAt: in.RemindAt, CompletedAt: completed, AssigneeID: in.AssigneeID,
		EntityType: in.EntityType, EntityID: in.EntityID}, nil
}

type taskView struct {
	store.Task
	AssigneeName *string `json:"assignee_name,omitempty"`
	Overdue      bool    `json:"overdue"`
}

func taskOverdue(t store.Task) bool {
	return t.DueAt != nil && t.DueAt.Before(time.Now()) && (t.Status == "open" || t.Status == "in_progress")
}

func (s *Server) listTasks(r *http.Request, limit, offset int32) ([]taskView, int64, error) {
	var assignee *uuid.UUID
	if r.URL.Query().Get("assignee_id") == "me" {
		assignee = userIDPtr(r)
	} else {
		var err error
		if assignee, err = qUUID(r, "assignee_id"); err != nil {
			return nil, 0, err
		}
	}
	entityID, err := qUUID(r, "entity_id")
	if err != nil {
		return nil, 0, err
	}
	dueBefore, err := qTime(r, "due_before")
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.db.ListTasks(r.Context(), store.ListTasksParams{Q: qText(r, "q"), Status: qText(r, "status"), OpenOnly: qBool(r, "open_only"),
		AssigneeID: assignee, EntityType: qText(r, "entity_type"), EntityID: entityID, DueBefore: dueBefore,
		TaskType: qText(r, "task_type"), Sort: qSort(r, "due", "-created"), Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	out := make([]taskView, 0, len(rows))
	var total int64
	for _, row := range rows {
		out = append(out, taskView{Task: row.Task, AssigneeName: row.AssigneeName, Overdue: taskOverdue(row.Task)})
		total = row.Total
	}
	return out, total, nil
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) error {
	limit, offset := pagination(r)
	items, total, err := s.listTasks(r, limit, offset)
	if err != nil {
		return err
	}
	return ok(w, Page[taskView]{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) handleTaskReminders(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.DueReminders(r.Context(), userIDPtr(r))
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	t, err := s.db.GetTask(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, taskView{Task: t, Overdue: taskOverdue(t)})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) error {
	var in taskInput
	if err := decode(r, &in); err != nil {
		return err
	}
	p, err := s.prepareTask(r, &in, nil)
	if err != nil {
		return err
	}
	var t store.Task
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		t, err = q.CreateTask(r.Context(), store.CreateTaskParams{Title: p.Title, Description: p.Description, Status: p.Status,
			Priority: p.Priority, TaskType: p.TaskType, DueAt: p.DueAt, RemindAt: p.RemindAt, CompletedAt: p.CompletedAt,
			AssigneeID: p.AssigneeID, EntityType: p.EntityType, EntityID: p.EntityID, CreatedBy: userIDPtr(r)})
		if err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "task.create", EntityType: deref(p.EntityType), EntityID: p.EntityID,
			Summary: "Created task " + t.Title, After: t})
	})
	if err != nil {
		return err
	}
	return created(w, taskView{Task: t, Overdue: taskOverdue(t)})
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in taskInput
	if err := decode(r, &in); err != nil {
		return err
	}
	before, err := s.db.GetTask(r.Context(), id)
	if err != nil {
		return err
	}
	p, err := s.prepareTask(r, &in, &before)
	if err != nil {
		return err
	}
	p.ID = id
	var t store.Task
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		if t, err = q.UpdateTask(r.Context(), p); err != nil {
			return err
		}
		action := "task.update"
		if before.Status != "done" && t.Status == "done" {
			action = "task.complete"
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: action, EntityType: deref(t.EntityType), EntityID: t.EntityID,
			Summary: "Updated task " + t.Title, Before: before, After: t})
	})
	if err != nil {
		return err
	}
	return ok(w, taskView{Task: t, Overdue: taskOverdue(t)})
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	err = s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetTask(r.Context(), id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteTask(r.Context(), id); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "task.delete", EntityType: deref(before.EntityType), EntityID: before.EntityID,
			Summary: "Deleted task " + before.Title, Before: before})
	})
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) taskBulkActions() map[string]bulkAction {
	return map[string]bulkAction{
		"delete": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, _ *string) (int64, error) {
			return q.DeleteTasksBulk(ctx, ids)
		}},
		"set_status": {auth.PermCRMWrite, func(ctx context.Context, q *store.Queries, ids []uuid.UUID, v *string) (int64, error) {
			vv := valid.New()
			vv.OneOf("value", deref(v), taskStatuses...)
			if err := checkValid(vv); err != nil {
				return 0, err
			}
			return q.SetTaskStatusBulk(ctx, store.SetTaskStatusBulkParams{Status: *v, Ids: ids})
		}},
	}
}

// ---- notes ----

func (s *Server) entityFromQuery(r *http.Request, write bool) (string, uuid.UUID, error) {
	et := r.URL.Query().Get("entity_type")
	id, err := qUUID(r, "entity_id")
	if err != nil {
		return "", uuid.Nil, err
	}
	if et == "" || id == nil {
		return "", uuid.Nil, errBadRequest("entity_type and entity_id are required.")
	}
	return et, *id, s.requireEntity(r, et, *id, write)
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) error {
	et, id, err := s.entityFromQuery(r, false)
	if err != nil {
		return err
	}
	rows, err := s.db.ListNotes(r.Context(), store.ListNotesParams{EntityType: et, EntityID: id})
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type noteInput struct {
	EntityType string     `json:"entity_type"`
	EntityID   uuid.UUID  `json:"entity_id"`
	ParentID   *uuid.UUID `json:"parent_id"`
	Body       string     `json:"body"`
	IsPinned   bool       `json:"is_pinned"`
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) error {
	var in noteInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Body = normalize.MultilineText(in.Body)
	v := valid.New()
	v.Len("body", in.Body, 1, 50000)
	if err := checkValid(v); err != nil {
		return err
	}
	if err := s.requireEntity(r, in.EntityType, in.EntityID, true); err != nil {
		return err
	}
	if in.ParentID != nil {
		parent, err := s.db.GetNote(r.Context(), *in.ParentID)
		if err != nil || parent.EntityType != in.EntityType || parent.EntityID != in.EntityID {
			return errField("parent_id", "must be a note on the same record")
		}
	}
	var n store.Note
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		var err error
		n, err = q.CreateNote(r.Context(), store.CreateNoteParams{EntityType: in.EntityType, EntityID: in.EntityID, ParentID: in.ParentID,
			Body: in.Body, IsPinned: in.IsPinned, AuthorID: userIDPtr(r)})
		if err != nil {
			return err
		}
		kind := "note"
		if in.ParentID != nil {
			kind = "comment"
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "note.create", EntityType: in.EntityType, EntityID: &in.EntityID,
			Summary: "Added a " + kind + ": " + normalize.Truncate(strings.ReplaceAll(in.Body, "\n", " "), 120)})
	})
	if err != nil {
		return err
	}
	return created(w, n)
}

type noteUpdateInput struct {
	Body     string `json:"body"`
	IsPinned bool   `json:"is_pinned"`
}

func (s *Server) noteForEdit(r *http.Request) (store.Note, error) {
	id, err := pathUUID(r, "id")
	if err != nil {
		return store.Note{}, err
	}
	n, err := s.db.GetNote(r.Context(), id)
	if err != nil {
		return n, errNotFound("Note")
	}
	info := entities[n.EntityType]
	p := principal(r)
	isAuthor := n.AuthorID != nil && *n.AuthorID == p.User.ID
	if !(isAuthor && p.Can(info.writePerm)) && !p.Can(info.delPerm) {
		return n, errForbidden("Only the author or a user who can delete records may change this note.")
	}
	return n, nil
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) error {
	n, err := s.noteForEdit(r)
	if err != nil {
		return err
	}
	var in noteUpdateInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Body = normalize.MultilineText(in.Body)
	if in.Body == "" || len(in.Body) > 50000 {
		return errField("body", "must be between 1 and 50000 characters")
	}
	updated, err := s.db.UpdateNote(r.Context(), store.UpdateNoteParams{ID: n.ID, Body: in.Body, IsPinned: in.IsPinned})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "note.update", EntityType: n.EntityType, EntityID: &n.EntityID, Summary: "Edited a note"})
	return ok(w, updated)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) error {
	n, err := s.noteForEdit(r)
	if err != nil {
		return err
	}
	if _, err := s.db.DeleteNote(r.Context(), n.ID); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "note.delete", EntityType: n.EntityType, EntityID: &n.EntityID, Summary: "Deleted a note"})
	return noContent(w)
}

// ---- attachments ----

// allowedUploadTypes maps sniffed content types we accept. Anything else is stored as
// application/octet-stream and always downloaded, never rendered inline.
var inlineSafeTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "application/pdf": true}

var blockedExtensions = map[string]bool{".exe": true, ".dll": true, ".bat": true, ".cmd": true, ".com": true, ".msi": true,
	".scr": true, ".ps1": true, ".vbs": true, ".jar": true, ".apk": true, ".app": true, ".sh": false}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSpace(strings.Trim(name, "."))
	if name == "" {
		name = "file"
	}
	return normalize.Truncate(name, 200)
}

// saveUpload stores the "file" part of a multipart request and returns the attachment metadata.
func (s *Server) saveUpload(r *http.Request, entityType *string, entityID *uuid.UUID) (store.Attachment, string, map[string]string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, s.cfg.MaxUploadBytes+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		return store.Attachment{}, "", nil, &APIError{Status: http.StatusUnsupportedMediaType, Code: "unsupported_media_type", Message: "Upload files as multipart/form-data."}
	}
	fields := map[string]string{}
	var saved filestore.Saved
	var filename, sniffed string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return store.Attachment{}, "", nil, &APIError{Status: http.StatusRequestEntityTooLarge, Code: "file_too_large", Message: fmt.Sprintf("Files are limited to %d MB.", s.cfg.MaxUploadBytes>>20)}
			}
			return store.Attachment{}, "", nil, errBadRequest("The upload could not be read.")
		}
		if part.FormName() != "file" {
			b, _ := io.ReadAll(io.LimitReader(part, 64<<10))
			fields[part.FormName()] = string(b)
			continue
		}
		if filename != "" {
			return store.Attachment{}, "", nil, errBadRequest("Upload one file at a time.")
		}
		filename = sanitizeFilename(part.FileName())
		if blockedExtensions[strings.ToLower(filepath.Ext(filename))] {
			return store.Attachment{}, "", nil, errField("file", "executable files are not accepted")
		}
		head := make([]byte, 512)
		n, _ := io.ReadFull(part, head)
		head = head[:n]
		sniffed = http.DetectContentType(head)
		saved, err = s.files.Save(io.MultiReader(strings.NewReader(string(head)), part), s.cfg.MaxUploadBytes)
		if errors.Is(err, filestore.ErrTooLarge) {
			return store.Attachment{}, "", nil, &APIError{Status: http.StatusRequestEntityTooLarge, Code: "file_too_large", Message: fmt.Sprintf("Files are limited to %d MB.", s.cfg.MaxUploadBytes>>20)}
		}
		if err != nil {
			return store.Attachment{}, "", nil, err
		}
	}
	if filename == "" {
		return store.Attachment{}, "", nil, errField("file", "is required")
	}
	ct := sniffed
	if mt, _, err := mime.ParseMediaType(sniffed); err == nil {
		ct = mt
	}
	if ext := strings.ToLower(filepath.Ext(filename)); ct == "text/plain" || ct == "application/octet-stream" {
		if byExt := mime.TypeByExtension(ext); byExt != "" && !strings.HasPrefix(byExt, "text/html") && !strings.Contains(byExt, "javascript") {
			if mt, _, err := mime.ParseMediaType(byExt); err == nil {
				ct = mt
			}
		}
	}
	if strings.HasPrefix(ct, "text/html") || strings.Contains(ct, "svg") || strings.Contains(ct, "javascript") || strings.Contains(ct, "xml") {
		ct = "text/plain" // never serve active content from this origin
	}
	if et, ok := fields["entity_type"]; ok && entityType == nil && et != "" {
		entityType = &et
		id, err := uuid.Parse(fields["entity_id"])
		if err != nil {
			_ = s.cleanupKey(r, saved.Key)
			return store.Attachment{}, "", nil, errField("entity_id", "must be a UUID")
		}
		entityID = &id
	}
	if entityType != nil {
		if err := s.requireEntity(r, *entityType, *entityID, true); err != nil {
			_ = s.cleanupKey(r, saved.Key)
			return store.Attachment{}, "", nil, err
		}
	}
	a, err := s.db.CreateAttachment(r.Context(), store.CreateAttachmentParams{EntityType: entityType, EntityID: entityID,
		Filename: filename, ContentType: ct, SizeBytes: saved.Size, Sha256: saved.SHA256, StorageKey: saved.Key, UploadedBy: userIDPtr(r)})
	if err != nil {
		_ = s.cleanupKey(r, saved.Key)
		return a, "", nil, err
	}
	return a, sniffed, fields, nil
}

func (s *Server) cleanupKey(r *http.Request, key string) error {
	inUse, err := s.db.StorageKeyInUse(r.Context(), key)
	if err != nil || inUse {
		return err
	}
	return s.files.Delete(key)
}

func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) error {
	a, _, _, err := s.saveUpload(r, nil, nil)
	if err != nil {
		return err
	}
	if a.EntityType == nil {
		_ = s.cleanupAttachment(r, a)
		return errField("entity_type", "is required")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "attachment.upload", EntityType: *a.EntityType, EntityID: a.EntityID,
		Summary: "Uploaded " + a.Filename, Extra: map[string]any{"attachment_id": a.ID, "size_bytes": a.SizeBytes}})
	return created(w, a)
}

func (s *Server) cleanupAttachment(r *http.Request, a store.Attachment) error {
	if _, err := s.db.DeleteAttachment(r.Context(), a.ID); err != nil {
		return err
	}
	return s.cleanupKey(r, a.StorageKey)
}

func (s *Server) handleListAttachments(w http.ResponseWriter, r *http.Request) error {
	et, id, err := s.entityFromQuery(r, false)
	if err != nil {
		return err
	}
	rows, err := s.db.ListAttachments(r.Context(), store.ListAttachmentsParams{EntityType: &et, EntityID: &id})
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) attachmentForRequest(r *http.Request, write bool) (store.Attachment, error) {
	id, err := pathUUID(r, "id")
	if err != nil {
		return store.Attachment{}, err
	}
	a, err := s.db.GetAttachment(r.Context(), id)
	if err != nil {
		return a, errNotFound("Attachment")
	}
	if a.EntityType != nil {
		if err := s.requireEntity(r, *a.EntityType, *a.EntityID, write); err != nil {
			return a, err
		}
	} else if a.UploadedBy == nil || *a.UploadedBy != principal(r).User.ID {
		return a, errNotFound("Attachment")
	}
	return a, nil
}

func (s *Server) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) error {
	a, err := s.attachmentForRequest(r, false)
	if err != nil {
		return err
	}
	f, err := s.files.Open(a.StorageKey)
	if err != nil {
		return errNotFound("Attachment file")
	}
	defer f.Close()
	ct := a.ContentType
	disposition := "attachment"
	if inlineSafeTypes[ct] && r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	} else if !inlineSafeTypes[ct] && ct != "text/plain" && !strings.HasPrefix(ct, "application/") {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": a.Filename}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=0, no-store")
	http.ServeContent(w, r, "", a.CreatedAt, f)
	return nil
}

func (s *Server) handleDeleteAttachment(w http.ResponseWriter, r *http.Request) error {
	a, err := s.attachmentForRequest(r, true)
	if err != nil {
		return err
	}
	if err := s.cleanupAttachment(r, a); err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "attachment.delete", EntityType: deref(a.EntityType), EntityID: a.EntityID, Summary: "Deleted " + a.Filename})
	return noContent(w)
}

// ---- tags ----

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListTags(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type tagInput struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (in *tagInput) validate() error {
	in.Name = normalize.Text(in.Name)
	if in.Color == "" {
		in.Color = "#6366f1"
	}
	v := valid.New()
	v.Len("name", in.Name, 1, 60)
	v.Check(len(in.Color) == 7 && in.Color[0] == '#' && strings.Trim(strings.ToLower(in.Color[1:]), "0123456789abcdef") == "", "color", "must look like #1a2b3c")
	return checkValid(v)
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) error {
	var in tagInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	t, err := s.db.CreateTag(r.Context(), store.CreateTagParams{Name: in.Name, Color: in.Color})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "tag.create", EntityType: "tag", EntityID: &t.ID, Summary: "Created tag " + t.Name})
	return created(w, t)
}

func (s *Server) handleUpdateTag(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in tagInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	t, err := s.db.UpdateTag(r.Context(), store.UpdateTagParams{ID: id, Name: in.Name, Color: in.Color})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "tag.update", EntityType: "tag", EntityID: &id, Summary: "Updated tag " + t.Name})
	return ok(w, t)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	n, err := s.db.DeleteTag(r.Context(), id)
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound("Tag")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "tag.delete", EntityType: "tag", EntityID: &id, Summary: "Deleted a tag"})
	return noContent(w)
}

type recordTagsInput struct {
	EntityType string      `json:"entity_type"`
	EntityID   uuid.UUID   `json:"entity_id"`
	TagIDs     []uuid.UUID `json:"tag_ids"`
	TagNames   []string    `json:"tag_names"`
}

// handleSetRecordTags replaces a record's tags. Tags given by name are created when missing.
func (s *Server) handleSetRecordTags(w http.ResponseWriter, r *http.Request) error {
	var in recordTagsInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.requireEntity(r, in.EntityType, in.EntityID, true); err != nil {
		return err
	}
	if len(in.TagIDs)+len(in.TagNames) > 50 {
		return errField("tag_ids", "a record can have at most 50 tags")
	}
	err := s.db.Tx(r.Context(), func(q *store.Queries, _ pgx.Tx) error {
		ids := append([]uuid.UUID{}, in.TagIDs...)
		for _, name := range in.TagNames {
			name = normalize.Text(name)
			if name == "" || len(name) > 60 {
				return errField("tag_names", "tag names must be 1-60 characters")
			}
			t, err := q.UpsertTag(r.Context(), store.UpsertTagParams{Name: name, Color: "#6366f1"})
			if err != nil {
				return err
			}
			ids = append(ids, t.ID)
		}
		if err := setRecordTags(r.Context(), q, in.EntityType, in.EntityID, ids); err != nil {
			return err
		}
		return s.audit(r.Context(), q, r, auditEvent{Action: "tags.set", EntityType: in.EntityType, EntityID: &in.EntityID, Summary: "Updated tags"})
	})
	if err != nil {
		return err
	}
	tags, err := s.recordTags(r.Context(), in.EntityType, in.EntityID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": tags})
}

// ---- custom fields ----

func (s *Server) handleListCustomFields(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.ListCustomFields(r.Context(), qText(r, "entity_type"))
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type customFieldInput struct {
	EntityType string   `json:"entity_type"`
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	FieldType  string   `json:"field_type"`
	Options    []string `json:"options"`
	Required   bool     `json:"required"`
	Position   int32    `json:"position"`
}

func (in *customFieldInput) validate(creating bool) error {
	in.Label = normalize.Text(in.Label)
	opts := []string{}
	for _, o := range in.Options {
		if o = normalize.Text(o); o != "" {
			opts = append(opts, o)
		}
	}
	in.Options = opts
	v := valid.New()
	if creating {
		v.OneOf("entity_type", in.EntityType, "company", "contact", "supplier", "lead", "opportunity", "product")
		v.Check(len(in.Key) >= 1 && len(in.Key) <= 41 && in.Key[0] >= 'a' && in.Key[0] <= 'z' &&
			strings.Trim(in.Key, "abcdefghijklmnopqrstuvwxyz0123456789_") == "", "key", "must be lowercase letters, digits or underscores, starting with a letter")
	}
	v.Len("label", in.Label, 1, 80)
	v.OneOf("field_type", in.FieldType, "text", "number", "date", "boolean", "select", "url")
	if in.FieldType == "select" {
		v.Check(len(in.Options) >= 1 && len(in.Options) <= 100, "options", "select fields need between 1 and 100 options")
	}
	return checkValid(v)
}

func (s *Server) handleCreateCustomField(w http.ResponseWriter, r *http.Request) error {
	var in customFieldInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(true); err != nil {
		return err
	}
	cf, err := s.db.CreateCustomField(r.Context(), store.CreateCustomFieldParams{EntityType: in.EntityType, Key: in.Key, Label: in.Label,
		FieldType: in.FieldType, Options: in.Options, Required: in.Required, Position: in.Position})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "custom_field.create", EntityType: "custom_field", EntityID: &cf.ID, Summary: "Created custom field " + cf.Label})
	return created(w, cf)
}

func (s *Server) handleUpdateCustomField(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	var in customFieldInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(false); err != nil {
		return err
	}
	cf, err := s.db.UpdateCustomField(r.Context(), store.UpdateCustomFieldParams{ID: id, Label: in.Label, FieldType: in.FieldType,
		Options: in.Options, Required: in.Required, Position: in.Position})
	if err != nil {
		return err
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "custom_field.update", EntityType: "custom_field", EntityID: &id, Summary: "Updated custom field " + cf.Label})
	return ok(w, cf)
}

func (s *Server) handleDeleteCustomField(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	n, err := s.db.DeleteCustomField(r.Context(), id)
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound("Custom field")
	}
	_ = s.audit(r.Context(), nil, r, auditEvent{Action: "custom_field.delete", EntityType: "custom_field", EntityID: &id,
		Summary: "Deleted a custom field definition; stored values are kept on records"})
	return noContent(w)
}

// ---- saved filters ----

func (s *Server) handleListSavedFilters(w http.ResponseWriter, r *http.Request) error {
	et := r.URL.Query().Get("entity_type")
	if et == "" {
		return errBadRequest("entity_type is required.")
	}
	rows, err := s.db.ListSavedFilters(r.Context(), store.ListSavedFiltersParams{EntityType: et, UserID: principal(r).User.ID})
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

type savedFilterInput struct {
	EntityType string            `json:"entity_type"`
	Name       string            `json:"name"`
	Filters    map[string]string `json:"filters"`
	IsShared   bool              `json:"is_shared"`
}

func (s *Server) handleCreateSavedFilter(w http.ResponseWriter, r *http.Request) error {
	var in savedFilterInput
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Name = normalize.Text(in.Name)
	v := valid.New()
	_, known := entities[in.EntityType]
	v.Check(known || in.EntityType == "task" || in.EntityType == "product_import", "entity_type", "is not supported")
	v.Len("name", in.Name, 1, 80)
	v.Check(len(in.Filters) <= 30, "filters", "may contain at most 30 entries")
	for k, val := range in.Filters {
		v.Check(len(k) <= 40 && len(val) <= 200, "filters", "keys and values are too long")
	}
	if err := checkValid(v); err != nil {
		return err
	}
	raw, _ := json.Marshal(in.Filters)
	f, err := s.db.CreateSavedFilter(r.Context(), store.CreateSavedFilterParams{UserID: principal(r).User.ID, EntityType: in.EntityType,
		Name: in.Name, Filters: raw, IsShared: in.IsShared})
	if err != nil {
		return err
	}
	return created(w, f)
}

func (s *Server) handleDeleteSavedFilter(w http.ResponseWriter, r *http.Request) error {
	id, err := pathUUID(r, "id")
	if err != nil {
		return err
	}
	n, err := s.db.DeleteSavedFilter(r.Context(), store.DeleteSavedFilterParams{ID: id, UserID: principal(r).User.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound("Saved filter")
	}
	return noContent(w)
}
