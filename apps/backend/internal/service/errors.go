package service

import "errors"

var (
	ErrInvalidProfile    = errors.New("invalid profile")
	ErrInvalidProfession = errors.New("invalid profession")
	ErrItemNotActive     = errors.New("checklist item is not active")

	ErrUserNotIntern             = errors.New("user is not an intern")
	ErrUserNotBuddy              = errors.New("user is not a buddy")
	ErrUserNotTeamlead           = errors.New("user is not a teamlead")
	ErrUserNotStaff              = errors.New("user is not staff")
	ErrInternNotAssigned         = errors.New("intern is not assigned to this buddy")
	ErrCannotReviewSelf          = errors.New("buddy cannot review their own checklist")
	ErrReviewNotPending          = errors.New("checklist item is not awaiting review")
	ErrSubmissionRequired        = errors.New("checklist item requires a submission")
	ErrInvalidSubmission         = errors.New("invalid checklist item submission")
	ErrSubmissionAlreadyReviewed = errors.New("checklist item submission was already reviewed")
	ErrProfessionNotSet          = errors.New("user profession is not set")
	ErrProgressIncomplete        = errors.New("user progress is incomplete")
	ErrOutdatedProfessionTrack   = errors.New("professional checklist no longer matches user profession")
)
