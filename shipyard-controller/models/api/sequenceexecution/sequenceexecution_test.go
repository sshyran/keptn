package sequenceexecution

import (
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	"github.com/keptn/keptn/shipyard-controller/models"
	"reflect"
	"testing"
	"time"
)

func TestFromSequenceExecutions(t *testing.T) {

	timestamp := time.Now()
	type args struct {
		executions []models.SequenceExecution
	}
	tests := []struct {
		name    string
		args    args
		want    *SequenceExecutions
		wantErr bool
	}{
		{
			name: "",
			args: args{
				executions: []models.SequenceExecution{
					{
						Status: models.SequenceExecutionStatus{
							State: models.SequenceStartedState,
							PreviousTasks: []models.TaskExecutionResult{
								{
									Name:   "deployment",
									Result: keptnv2.ResultPass,
									Status: keptnv2.StatusSucceeded,
								},
								{
									Name:   "evaluation",
									Result: keptnv2.ResultPass,
									Status: keptnv2.StatusSucceeded,
									Properties: map[string]interface{}{
										"evaluation": map[string]interface{}{
											"score": 100.0,
										},
									},
								},
							},
							CurrentTask: models.TaskExecutionState{
								Name: "release",
							},
						},
						Scope: models.EventScope{
							EventData: keptnv2.EventData{
								Project: "my-project",
								Stage:   "my-stage-1",
								Service: "my-service",
							},
							KeptnContext: "my-context",
						},
						Timestamp: timestamp,
					},
				},
			},
			want: &SequenceExecutions{
				Scope: models.EventScope{
					EventData: keptnv2.EventData{
						Project: "my-project",
						Service: "my-service",
					},
					KeptnContext: "my-context",
				},
				SequenceExecutions: []SequenceExecution{
					{
						Status: ExecutionStatus{},
						Scope: models.EventScope{
							EventData: keptnv2.EventData{
								Project: "my-project",
								Stage:   "my-stage-1",
								Service: "my-service",
							},
							KeptnContext: "my-context",
						},
					},
					{
						Status: ExecutionStatus{},
						Scope: models.EventScope{
							EventData: keptnv2.EventData{
								Project: "my-project",
								Stage:   "my-stage-2",
								Service: "my-service",
							},
							KeptnContext: "my-context",
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromSequenceExecutions(tt.args.executions)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromSequenceExecutions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromSequenceExecutions() got = %v, want %v", got, tt.want)
			}
		})
	}
}
