package sequenceexecution

import (
	"errors"
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	"github.com/keptn/keptn/shipyard-controller/models"
	"time"
)

var ErrEmptySequenceExecutions = errors.New("provided list of sequence executions is empty")
var ErrSequenceExecutionsScopeMismatch = errors.New("sequence executions do not belong to the same scope")

// TaskExecutionState describes the currently active task of a sequence execution
type TaskExecutionState struct {
	Name string `json:"name" bson:"name"`
}

// EvaluationTaskResult contains specific information about the result of an evaluation task
type EvaluationTaskResult struct {
	Score float64 `json:"score"`
}

// TaskExecutionResult describes the result of a completed task within a sequence execution
type TaskExecutionResult struct {
	Name             string                `json:"name"`
	TriggeredID      string                `json:"triggeredID"`
	Result           keptnv2.ResultType    `json:"result"`
	Status           keptnv2.StatusType    `json:"status"`
	EvaluationResult *EvaluationTaskResult `json:"evaluationResult,omitempty"`
}

// ExecutionStatus describes the current state of a sequence execution
type ExecutionStatus struct {
	State     string `json:"state"`
	Timestamp time.Time
	// PreviousTasks contains the results of all completed tasks of the sequence
	PreviousTasks []TaskExecutionResult `json:"previousTasks"`
	// CurrentTask represents the state of the currently active task - pointer because this property can also be nil (e.g. if the sequence is already finished)
	CurrentTask *TaskExecutionState `json:"currentTask,omitempty"`
}

// SequenceExecution represents the data model for sequence executions provided by the API
// This struct is introduced to decouple the internal SequenceExecution model from the API model
type SequenceExecution struct {
	Status ExecutionStatus   `json:"status"`
	Scope  models.EventScope `json:"scope"`
}

// SequenceExecutions is an aggregation of sequences belonging to the same KeptnContext (e.g. multi-stage delivery, multiple iterations of a remediation sequence)
type SequenceExecutions struct {
	Scope              models.EventScope   `json:"scope"`
	SequenceExecutions []SequenceExecution `json:"sequenceExecutions"`
}

// FromSequenceExecutions is a mapping function that transforms the internal SequenceExecution model into the respective API model representation.
func FromSequenceExecutions(executions []models.SequenceExecution) (*SequenceExecutions, error) {
	if len(executions) == 0 {
		return nil, ErrEmptySequenceExecutions
	}
	if !scopeMatches(executions) {
		return nil, ErrSequenceExecutionsScopeMismatch
	}
	aggregatedSequenceExecutions := &SequenceExecutions{
		Scope: models.EventScope{
			// Project and service are the same across all executions, but the stage can vary
			EventData: keptnv2.EventData{
				Project: executions[0].Scope.Project,
				Service: executions[0].Scope.Service,
			},
			KeptnContext: executions[0].Scope.KeptnContext,
		},
		SequenceExecutions: []SequenceExecution{},
	}

	for _, execution := range executions {
		newExecution := SequenceExecution{
			Status: ExecutionStatus{
				State:         execution.Status.State,
				Timestamp:     execution.Timestamp,
				PreviousTasks: []TaskExecutionResult{},
			},
			Scope: execution.Scope,
		}

		for _, previousTask := range execution.Status.PreviousTasks {
			taskResult := TaskExecutionResult{
				Name:             previousTask.Name,
				TriggeredID:      previousTask.TriggeredID,
				Result:           previousTask.Result,
				Status:           previousTask.Status,
				EvaluationResult: nil,
			}

			if taskResult.Name == keptnv2.EvaluationTaskName {
				evaluationFinishedData := &keptnv2.EvaluationFinishedEventData{}
				err := keptnv2.Decode(previousTask.Properties, evaluationFinishedData)
				// if converting into EvaluationFinishedEventData was successful, we can set the score for the evaluation
				if err == nil {
					taskResult.EvaluationResult = &EvaluationTaskResult{Score: evaluationFinishedData.Evaluation.Score}
				}
			}
			newExecution.Status.PreviousTasks = append(newExecution.Status.PreviousTasks, taskResult)
		}

		if execution.Status.CurrentTask.Name != "" {
			newExecution.Status.CurrentTask = &TaskExecutionState{
				Name: execution.Status.CurrentTask.Name,
			}
		}

		aggregatedSequenceExecutions.SequenceExecutions = append(aggregatedSequenceExecutions.SequenceExecutions, newExecution)
	}
	return aggregatedSequenceExecutions, nil
}

// scopeMatches checks if all items in the provided collection belong to the same keptnContext/project/service
// other attributes, such as the stage can vary, e.g. for a multi-stage sequence
func scopeMatches(executions []models.SequenceExecution) bool {
	if len(executions) == 0 {
		return true
	}

	keptnContext := executions[0].Scope.KeptnContext
	project := executions[0].Scope.Project
	service := executions[0].Scope.Service

	for _, execution := range executions {
		if execution.Scope.KeptnContext != keptnContext {
			return false
		}
		if execution.Scope.Project != project {
			return false
		}
		if execution.Scope.Service != service {
			return false
		}
	}

	return true
}
