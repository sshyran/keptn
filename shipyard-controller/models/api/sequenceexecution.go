package api

import (
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	"github.com/keptn/keptn/shipyard-controller/models"
)

// TaskExecutionState describes the currently active task of a sequence execution
type TaskExecutionState struct {
	Name        string `json:"name" bson:"name"`
	TriggeredID string `json:"triggeredID" bson:"triggeredID"`
}

// EvaluationTaskResult contains specific information about the result of an evaluation task
type EvaluationTaskResult struct {
	Score float32 `json:"score"`
}

// TaskExecutionResult describes the result of a completed task within a sequence execution
type TaskExecutionResult struct {
	Name        string             `json:"name"`
	TriggeredID string             `json:"triggeredID"`
	Result      keptnv2.ResultType `json:"result"`
	Status      keptnv2.StatusType `json:"status"`
	// TODO: how detailed should the evaluation result be?
	EvaluationResult *EvaluationTaskResult `json:"evaluationResult,omitempty"`
}

// SequenceExecutionStatus describes the current state of a sequence execution
type SequenceExecutionStatus struct {
	State string `json:"state"`
	// PreviousTasks contains the results of all completed tasks of the sequence
	PreviousTasks []TaskExecutionResult `json:"previousTasks"`
	// CurrentTask represents the state of the currently active task - pointer because this property can also be nil (e.g. if the sequence is already finished)
	CurrentTask *TaskExecutionState `json:"currentTask,omitempty"`
}

// SequenceExecution represents the data model for sequence executions provided by the API
// This struct is introduced to decouple the internal SequenceExecution model from the API model
type SequenceExecution struct {
	// TODO: sequence definition from shipyard file also needed?
	Status SequenceExecutionStatus `json:"status"`
	Scope  models.EventScope       `json:"scope"`
}

// SequenceExecutions is an aggregation of sequences belonging to the same KeptnContext (e.g. multi-stage delivery, multiple iterations of a remediation sequence)
type SequenceExecutions struct {
	Scope models.EventScope `json:"scope"`
}
