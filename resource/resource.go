package resource

import (
    klabels "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
    kwatch "github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
)

// Common resources interface
type Resources interface {
    List(selector klabels.Selector) ([]interface{}, error)
    Watch(label klabels.Selector) (kwatch.Interface, error)
}