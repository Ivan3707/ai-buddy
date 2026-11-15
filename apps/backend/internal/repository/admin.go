package repository

import (
	"context"
	"database/sql"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type AdminRepository struct{ db DBTX }

func NewAdminRepository(db DBTX) *AdminRepository { return &AdminRepository{db: db} }
func (r *AdminRepository) AnswerFeedback(ctx context.Context) (model.AnswerFeedback, error) {
	out := model.AnswerFeedback{Items: []model.AnswerFeedbackItem{}}
	if e := r.db.QueryRowContext(ctx, `SELECT count(*)FILTER(WHERE feedback='up'),count(*)FILTER(WHERE feedback='down')FROM chat_messages`).Scan(&out.Up, &out.Down); e != nil {
		return out, e
	}
	rows, e := r.db.QueryContext(ctx, `SELECT id::text,feedback_comment,created_at FROM chat_messages WHERE feedback='down' AND COALESCE(feedback_comment,'')<>'' ORDER BY created_at DESC LIMIT 50`)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v model.AnswerFeedbackItem
		if e := rows.Scan(&v.MessageID, &v.Comment, &v.CreatedAt); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, rows.Err()
}
func (r *AdminRepository) RestartOnboarding(ctx context.Context, userID string) error {
	res, e := r.db.ExecContext(ctx, `UPDATE users SET last_login_at=NULL WHERE id=$1::uuid AND role='intern'`, userID)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (r *AdminRepository) Accounts(ctx context.Context) ([]model.AdminAccount, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT u.id::text,u.email,COALESCE(u.first_name,''),COALESCE(u.last_name,''),u.role,COALESCE(u.profession,''),COALESCE(u.team,''),COALESCE(u.department,''),COALESCE(NULLIF(trim(COALESCE(b.first_name,'')||' '||COALESCE(b.last_name,'')),''),b.email,''),COALESCE(NULLIF(trim(COALESCE(t.first_name,'')||' '||COALESCE(t.last_name,'')),''),t.email,''),u.created_at,u.last_login_at,(SELECT count(*)FROM auth_sessions a WHERE a.user_id=u.id AND a.revoked_at IS NULL AND a.expires_at>now())FROM users u LEFT JOIN users b ON b.id=u.buddy_id LEFT JOIN users t ON t.id=u.teamlead_id ORDER BY CASE u.role WHEN 'teamlead'THEN 0 WHEN 'buddy'THEN 1 ELSE 2 END,u.created_at,u.email`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.AdminAccount{}
	for rows.Next() {
		var v model.AdminAccount
		if e := rows.Scan(&v.ID, &v.Email, &v.FirstName, &v.LastName, &v.Role, &v.Profession, &v.Team, &v.Department, &v.BuddyName, &v.TeamleadName, &v.CreatedAt, &v.LastLoginAt, &v.ActiveSessions); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *AdminRepository) GapQuestions(ctx context.Context) ([]map[string]string, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id::text,question FROM mentor_questions ORDER BY created_at DESC LIMIT 200`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var id, text string
		if e := rows.Scan(&id, &text); e != nil {
			return nil, e
		}
		out = append(out, map[string]string{"id": id, "text": text})
	}
	return out, rows.Err()
}
func (r *AdminRepository) HintCounts(ctx context.Context) (map[string]int, error) {
	out := map[string]int{"pending": 0, "processing": 0, "ready": 0, "unread": 0}
	rows, e := r.db.QueryContext(ctx, `SELECT status,count(*)FROM task_hints GROUP BY status`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var status string
		var n int
		if e := rows.Scan(&status, &n); e != nil {
			rows.Close()
			return out, e
		}
		out[status] = n
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	var unread int
	e = r.db.QueryRowContext(ctx, `SELECT count(*)FROM task_hints WHERE status='ready' AND read_at IS NULL`).Scan(&unread)
	out["unread"] = unread
	return out, e
}
func (r *AdminRepository) Ping(ctx context.Context) error {
	var one int
	return r.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one)
}
func (r *AdminRepository) Counters(ctx context.Context) (model.AdminCounters, error) {
	out := model.AdminCounters{UsersByRole: map[string]int{"intern": 0, "buddy": 0, "teamlead": 0}}
	rows, e := r.db.QueryContext(ctx, `SELECT role,count(*)FROM users GROUP BY role`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var role string
		var n int
		if e := rows.Scan(&role, &n); e != nil {
			rows.Close()
			return out, e
		}
		out.UsersByRole[role] = n
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	e = r.db.QueryRowContext(ctx, `SELECT(SELECT count(*)FROM mentor_questions WHERE status='open'),(SELECT count(*)FROM service_requests WHERE status='pending_approval'),(SELECT count(*)FROM chat_messages WHERE created_at>=date_trunc('day',now())),(SELECT count(*)FROM auth_sessions WHERE revoked_at IS NULL AND expires_at>now())`).Scan(&out.OpenQuestions, &out.PendingRequests, &out.MessagesToday, &out.ActiveSessions)
	return out, e
}

type NotificationCounts struct{ OpenQuestions, AnsweredQuestions, PendingRequests, OpenAccess, Overdue int }

const overdueFilter = `p.status='active' AND p.hidden_at IS NULL AND p.activated_at IS NOT NULL AND COALESCE(p.estimated_minutes_override,i.estimated_minutes)IS NOT NULL AND p.activated_at+COALESCE(p.estimated_minutes_override,i.estimated_minutes)*interval '1 minute'<now()`

func (r *AdminRepository) NotificationCounts(ctx context.Context) (NotificationCounts, error) {
	var v NotificationCounts
	e := r.db.QueryRowContext(ctx, `SELECT(SELECT count(*)FROM mentor_questions WHERE status='open'),(SELECT count(*)FROM mentor_questions WHERE status='answered'),(SELECT count(*)FROM service_requests WHERE status='pending_approval'),(SELECT count(*)FROM corporate_access_requests WHERE status='open'),(SELECT count(*)FROM user_checklist_progress p JOIN checklist_items i ON i.id=p.checklist_item_id WHERE `+overdueFilter+`)`).Scan(&v.OpenQuestions, &v.AnsweredQuestions, &v.PendingRequests, &v.OpenAccess, &v.Overdue)
	return v, e
}
func (r *AdminRepository) StuckTasks(ctx context.Context) ([]model.StuckTask, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT COALESCE(p.title_override,i.title),u.email,COALESCE(h.status,'none'),COALESCE(h.text,'')FROM user_checklist_progress p JOIN checklist_items i ON i.id=p.checklist_item_id JOIN users u ON u.id=p.user_id LEFT JOIN task_hints h ON h.progress_id=p.id WHERE `+overdueFilter+` ORDER BY p.activated_at LIMIT 10`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.StuckTask{}
	for rows.Next() {
		var v model.StuckTask
		if e := rows.Scan(&v.Task, &v.UserEmail, &v.Hint, &v.HintText); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
