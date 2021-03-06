package domain

import (
	"context"
	"github.com/streadway/amqp"
)

// Student struct
type Student struct {
	ID        string `json:"id"` //uuid string
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	IsPending bool    `json:"isPending"`
}

// StudentRepository implements the contract for student repository. We only save and get a student here
type StudentRepository interface {
	SaveStudent(ctx context.Context, student *Student) error
	GetStudent(ctx context.Context, studentID string) (*Student, error)
	EditStudent(ctx context.Context, student *Student) error
	DeleteStudent(ctx context.Context, id string) error
}

// StudentUseCase implements the contract for student functionalities. GetStudent is used by the app, but
// ListenAndSyncStudentRecord is used to sync the data from the profile service
type StudentUseCase interface {
	ListenStudentCreation(channel *amqp.Channel)
	ListenStudentEdit(channel *amqp.Channel)
	ListenStudentDelete(channel *amqp.Channel)
}
