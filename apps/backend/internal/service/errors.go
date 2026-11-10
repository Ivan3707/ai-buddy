package service

import "errors"

var (
	ErrInvalidProfession = errors.New("invalid profession")
	ErrItemNotActive     = errors.New("checklist item is not active")

	ErrUserNotIntern      = errors.New("user is not an intern")
	ErrUserNotBuddy       = errors.New("user is not a buddy")
	ErrInternNotAssigned  = errors.New("intern is not assigned to this buddy")
	ErrCannotReviewSelf   = errors.New("buddy cannot review their own checklist")
	ErrReviewNotPending   = errors.New("checklist item is not awaiting review")
	ErrProfessionNotSet   = errors.New("user profession is not set")
	ErrProgressIncomplete = errors.New("user progress is incomplete")
)
