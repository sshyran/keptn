package handler

import "github.com/keptn/keptn/shipyard-controller/models"

type ISequenceExecutionHandler interface {
	GetSequenceExecutions(filter models.SequenceExecutionFilter)
}
