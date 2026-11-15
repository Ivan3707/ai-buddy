package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type AssistantRepository struct{ db DBTX }

func NewAssistantRepository(db DBTX) *AssistantRepository { return &AssistantRepository{db: db} }

func (r *AssistantRepository) Conversation(ctx context.Context, userID uuid.UUID, requested *uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	if requested != nil {
		err := r.db.QueryRowContext(ctx, `SELECT id FROM chat_conversations WHERE id=$1 AND user_id=$2`, *requested, userID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return id, err
	}
	err := r.db.QueryRowContext(ctx, `INSERT INTO chat_conversations(user_id) VALUES($1) RETURNING id`, userID).Scan(&id)
	return id, err
}

func (r *AssistantRepository) LatestConversation(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT id FROM chat_conversations WHERE user_id=$1 ORDER BY updated_at DESC LIMIT 1`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (r *AssistantRepository) History(ctx context.Context, conversationID uuid.UUID, limit int) ([]model.ChatTurn, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT role,content FROM (SELECT role,content,created_at FROM chat_messages WHERE conversation_id=$1 ORDER BY created_at DESC LIMIT $2) last ORDER BY created_at`, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}
	defer rows.Close()
	out := []model.ChatTurn{}
	for rows.Next() {
		var v model.ChatTurn
		if err := rows.Scan(&v.Role, &v.Content); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *AssistantRepository) AddUserMessage(ctx context.Context, conversationID uuid.UUID, content string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO chat_messages(conversation_id,role,content,created_at) VALUES($1,'user',$2,clock_timestamp())`, conversationID, content)
	return err
}

func (r *AssistantRepository) AddAssistantMessage(ctx context.Context, id, conversationID uuid.UUID, content string, sources json.RawMessage, actionID *uuid.UUID, finish *string) error {
	var src any
	if len(sources) > 0 {
		src = []byte(sources)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO chat_messages(id,conversation_id,role,content,sources,action_id,finish_reason) VALUES($1,$2,'assistant',$3,$4,$5,$6)`, id, conversationID, content, src, actionID, finish)
	return err
}

func (r *AssistantRepository) TouchConversation(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE chat_conversations SET updated_at=now() WHERE id=$1`, id)
	return err
}

func (r *AssistantRepository) StoreProposal(ctx context.Context, userID uuid.UUID, kind string, params json.RawMessage, summary string) (model.PendingAction, error) {
	var a model.PendingAction
	err := r.db.QueryRowContext(ctx, `INSERT INTO assistant_actions(user_id,type,params,summary) VALUES($1,$2,$3,$4) RETURNING id,type,params,summary,status,error,created_at`, userID, kind, []byte(params), summary).Scan(&a.ID, &a.Type, &a.Params, &a.Summary, &a.Status, &a.Error, &a.CreatedAt)
	return a, err
}

func (r *AssistantRepository) Messages(ctx context.Context, conversationID uuid.UUID) ([]model.ChatMessage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT m.id,m.role,m.content,m.sources,m.finish_reason,m.feedback,m.created_at,a.id,a.type,a.params,a.summary,a.status,a.error,a.created_at FROM chat_messages m LEFT JOIN assistant_actions a ON a.id=m.action_id WHERE m.conversation_id=$1 ORDER BY m.created_at`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	out := []model.ChatMessage{}
	for rows.Next() {
		var m model.ChatMessage
		var sources []byte
		var aID *uuid.UUID
		var typ, summary, status *string
		var params []byte
		var actionErr *string
		var created *time.Time
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &sources, &m.FinishReason, &m.Feedback, &m.CreatedAt, &aID, &typ, &params, &summary, &status, &actionErr, &created); err != nil {
			return nil, err
		}
		if len(sources) > 0 {
			m.Sources = sources
		}
		if aID != nil {
			m.Action = &model.PendingAction{ID: *aID, Type: *typ, Params: params, Summary: *summary, Status: *status, Error: actionErr, CreatedAt: *created}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *AssistantRepository) SetFeedback(ctx context.Context, userID, messageID uuid.UUID, rating, comment string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE chat_messages m SET feedback_comment=CASE WHEN $4<>'' THEN $4 WHEN m.feedback IS DISTINCT FROM $3 THEN NULL ELSE m.feedback_comment END,feedback=$3 FROM chat_conversations c WHERE m.id=$1 AND c.id=m.conversation_id AND c.user_id=$2 AND m.role='assistant'`, messageID, userID, rating, comment)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AssistantRepository) Action(ctx context.Context, userID, actionID uuid.UUID, lock bool) (model.PendingAction, json.RawMessage, error) {
	q := `SELECT id,type,params,summary,status,error,created_at,result FROM assistant_actions WHERE id=$1 AND user_id=$2`
	if lock {
		q += ` FOR UPDATE`
	}
	var a model.PendingAction
	var result []byte
	err := r.db.QueryRowContext(ctx, q, actionID, userID).Scan(&a.ID, &a.Type, &a.Params, &a.Summary, &a.Status, &a.Error, &a.CreatedAt, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return a, nil, ErrNotFound
	}
	return a, result, err
}

func (r *AssistantRepository) FailAction(ctx context.Context, id uuid.UUID, message string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE assistant_actions SET status='failed',error=$2,updated_at=now() WHERE id=$1`, id, message)
	return err
}
func (r *AssistantRepository) CompleteAction(ctx context.Context, id uuid.UUID, result []byte) error {
	_, err := r.db.ExecContext(ctx, `UPDATE assistant_actions SET status='executed',result=$2,updated_at=now() WHERE id=$1`, id, result)
	return err
}
func (r *AssistantRepository) CancelAction(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE assistant_actions SET status='cancelled',updated_at=now() WHERE id=$1`, id)
	return err
}

func (r *AssistantRepository) CreateMentorQuestion(ctx context.Context, internID uuid.UUID, buddyID *uuid.UUID, actionID uuid.UUID, question, contextText string) (model.MentorQuestion, error) {
	q := model.MentorQuestion{InternID: internID, Question: question, Context: contextText, Status: "open", EscalationLevel: "mentor"}
	err := r.db.QueryRowContext(ctx, `INSERT INTO mentor_questions(intern_id,buddy_id,action_id,question,context) VALUES($1,$2,$3,$4,$5) RETURNING id,created_at`, internID, buddyID, actionID, question, contextText).Scan(&q.ID, &q.CreatedAt)
	q.EscalatedAt = q.CreatedAt
	return q, err
}

func (r *AssistantRepository) CreateServiceRequest(ctx context.Context, userID, actionID uuid.UUID, prefix, kind, approver string, params json.RawMessage) (model.ServiceRequest, error) {
	var v model.ServiceRequest
	err := r.db.QueryRowContext(ctx, `INSERT INTO service_requests(user_id,action_id,number,type,approver,params) VALUES($1,$2,$3||'-'||lpad(nextval('service_request_number_seq')::text,6,'0'),$4,$5,$6) RETURNING id,number,type,status,approver,params,created_at,updated_at`, userID, actionID, prefix, kind, approver, []byte(params)).Scan(&v.ID, &v.Number, &v.Type, &v.Status, &v.Approver, &v.Params, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func (r *AssistantRepository) UserName(ctx context.Context, id uuid.UUID) (*string, *string, error) {
	var first, last *string
	err := r.db.QueryRowContext(ctx, `SELECT first_name,last_name FROM users WHERE id=$1`, id).Scan(&first, &last)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return first, last, err
}

const requestColumns = `id,number,type,status,COALESCE(approver,''),COALESCE(decision_comment,''),params,created_at,updated_at`

func scanServiceRequest(row interface{ Scan(...any) error }) (model.ServiceRequest, error) {
	var v model.ServiceRequest
	err := row.Scan(&v.ID, &v.Number, &v.Type, &v.Status, &v.Approver, &v.DecisionComment, &v.Params, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func (r *AssistantRepository) ListRequests(ctx context.Context, userID uuid.UUID, limit int) ([]model.ServiceRequest, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+requestColumns+` FROM service_requests WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list requests: %w", err)
	}
	defer rows.Close()
	out := []model.ServiceRequest{}
	for rows.Next() {
		v, e := scanServiceRequest(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *AssistantRepository) RequestForOwner(ctx context.Context, id, userID uuid.UUID) (model.ServiceRequest, error) {
	v, e := scanServiceRequest(r.db.QueryRowContext(ctx, `SELECT `+requestColumns+` FROM service_requests WHERE id=$1 AND user_id=$2`, id, userID))
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrNotFound
	}
	return v, e
}
func (r *AssistantRepository) RequestForTeamlead(ctx context.Context, id, teamleadID uuid.UUID) (model.ServiceRequest, error) {
	v, e := scanServiceRequest(r.db.QueryRowContext(ctx, `SELECT r.id,r.number,r.type,r.status,COALESCE(r.approver,''),COALESCE(r.decision_comment,''),r.params,r.created_at,r.updated_at FROM service_requests r JOIN users u ON u.id=r.user_id WHERE r.id=$1 AND u.teamlead_id=$2`, id, teamleadID))
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrNotFound
	}
	return v, e
}
func (r *AssistantRepository) RequesterName(ctx context.Context, id uuid.UUID) (string, error) {
	var name string
	e := r.db.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(btrim(COALESCE(u.first_name,'')||' '||COALESCE(u.last_name,'')),''),u.email) FROM service_requests r JOIN users u ON u.id=r.user_id WHERE r.id=$1`, id).Scan(&name)
	return name, e
}
func (r *AssistantRepository) DecideRequest(ctx context.Context, id, teamleadID uuid.UUID, status, approver, comment string) (model.ServiceRequest, error) {
	v, e := scanServiceRequest(r.db.QueryRowContext(ctx, `UPDATE service_requests r SET status=$3,approver=$4,decision_comment=NULLIF($5,''),updated_at=now() FROM users u WHERE r.id=$1 AND u.id=r.user_id AND u.teamlead_id=$2 AND r.status='pending_approval' RETURNING r.id,r.number,r.type,r.status,COALESCE(r.approver,''),COALESCE(r.decision_comment,''),r.params,r.created_at,r.updated_at`, id, teamleadID, status, approver, comment))
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrNotFound
	}
	return v, e
}
func (r *AssistantRepository) TeamleadOwnsRequest(ctx context.Context, id, teamleadID uuid.UUID) (bool, error) {
	var ok bool
	e := r.db.QueryRowContext(ctx, `SELECT true FROM service_requests r JOIN users u ON u.id=r.user_id WHERE r.id=$1 AND u.teamlead_id=$2`, id, teamleadID).Scan(&ok)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	return ok, e
}

type StaffRequest struct {
	Request    model.ServiceRequest
	InternID   uuid.UUID
	InternName string
}

func (r *AssistantRepository) StaffRequests(ctx context.Context, teamleadID uuid.UUID) ([]StaffRequest, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT r.id,r.number,r.type,r.status,COALESCE(r.approver,''),COALESCE(r.decision_comment,''),r.params,r.created_at,r.updated_at,u.id,COALESCE(NULLIF(btrim(COALESCE(u.first_name,'')||' '||COALESCE(u.last_name,'')),''),u.email) FROM service_requests r JOIN users u ON u.id=r.user_id WHERE u.teamlead_id=$1 ORDER BY r.created_at DESC LIMIT 100`, teamleadID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []StaffRequest{}
	for rows.Next() {
		var v StaffRequest
		if e := rows.Scan(&v.Request.ID, &v.Request.Number, &v.Request.Type, &v.Request.Status, &v.Request.Approver, &v.Request.DecisionComment, &v.Request.Params, &v.Request.CreatedAt, &v.Request.UpdatedAt, &v.InternID, &v.InternName); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *AssistantRepository) Metrics(ctx context.Context, staffID uuid.UUID) (model.ChatMetrics, error) {
	var m model.ChatMetrics
	e := r.db.QueryRowContext(ctx, `WITH interns AS (SELECT id FROM users WHERE buddy_id=$1 OR teamlead_id=$1),msgs AS (SELECT m.* FROM chat_messages m JOIN chat_conversations c ON c.id=m.conversation_id WHERE c.user_id IN(SELECT id FROM interns)) SELECT (SELECT count(*) FROM msgs WHERE role='user'),(SELECT count(*) FROM mentor_questions WHERE intern_id IN(SELECT id FROM interns)),(SELECT count(*) FROM assistant_actions WHERE status='executed' AND user_id IN(SELECT id FROM interns))`, staffID).Scan(&m.QuestionsTotal, &m.EscalatedToMentor, &m.ActionsExecuted)
	return m, e
}

func (r *AssistantRepository) Tasks(ctx context.Context, userID uuid.UUID) ([]model.AssistantTask, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT i.id,COALESCE(p.title_override,i.title),COALESCE(p.description_override,i.description),p.status,c.type,st.title,st.position,i.requires_review,sub.status,sub.review_comment,(SELECT count(*) FROM checklist_stages x WHERE x.checklist_id=c.id) FROM user_checklist_progress p JOIN checklist_items i ON i.id=p.checklist_item_id JOIN checklists c ON c.id=p.checklist_id JOIN checklist_stages st ON st.id=i.stage_id LEFT JOIN LATERAL(SELECT s.status,s.review_comment FROM checklist_item_submissions s WHERE s.progress_id=p.id ORDER BY s.attempt_number DESC LIMIT 1)sub ON true WHERE p.user_id=$1 AND p.hidden_at IS NULL ORDER BY c.type DESC,st.position,p.position`, userID)
	if e != nil {
		return nil, fmt.Errorf("load tasks: %w", e)
	}
	defer rows.Close()
	out := []model.AssistantTask{}
	for rows.Next() {
		var v model.AssistantTask
		var sub, comment sql.NullString
		if e := rows.Scan(&v.ID, &v.Title, &v.Description, &v.Status, &v.Checklist, &v.StageTitle, &v.StagePosition, &v.RequiresResult, &sub, &comment, &v.StageTotal); e != nil {
			return nil, fmt.Errorf("scan task: %w", e)
		}
		if sub.Valid {
			v.Submission = &sub.String
		}
		if comment.Valid {
			v.ReviewComment = &comment.String
		}
		out = append(out, v)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("read tasks: %w", e)
	}
	return out, nil
}

func (r *AssistantRepository) AddHintTurn(ctx context.Context, conversationID uuid.UUID, question, text string, sources json.RawMessage) (uuid.UUID, error) {
	if _, e := r.db.ExecContext(ctx, `INSERT INTO chat_messages(conversation_id,role,content) VALUES($1,'user',$2)`, conversationID, question); e != nil {
		return uuid.Nil, e
	}
	var id uuid.UUID
	e := r.db.QueryRowContext(ctx, `INSERT INTO chat_messages(conversation_id,role,content,sources,finish_reason,created_at) VALUES($1,'assistant',$2,$3,'hint',clock_timestamp()) RETURNING id`, conversationID, text, []byte(sources)).Scan(&id)
	if e != nil {
		return uuid.Nil, e
	}
	return id, r.TouchConversation(ctx, conversationID)
}
func (r *AssistantRepository) AddMentorAnswer(ctx context.Context, id uuid.UUID, text string) error {
	_, e := r.db.ExecContext(ctx, `INSERT INTO chat_messages(conversation_id,role,content,finish_reason) SELECT m.conversation_id,'assistant',$2,'mentor_answer' FROM mentor_questions mq JOIN chat_messages m ON m.action_id=mq.action_id WHERE mq.id=$1 LIMIT 1`, id, text)
	return e
}
func (r *AssistantRepository) UserForContext(ctx context.Context, id uuid.UUID) (model.User, error) {
	var u model.User
	e := r.db.QueryRowContext(ctx, `SELECT id,role,profession,buddy_id,first_name,team FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Role, &u.Profession, &u.BuddyID, &u.FirstName, &u.Team)
	return u, e
}
