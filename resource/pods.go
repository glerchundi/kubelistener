package resource

import (
    kclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
    kfields "github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
    klabels "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
    kwatch "github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
)

type PodResources struct {
    pods kclient.PodInterface
}

func NewPodResources(ps kclient.PodInterface) PodResources {
    return PodResources{ps}
}

func (pr PodResources) List(selector klabels.Selector) ([]interface{}, error) {
    if pl, err := pr.pods.List(selector); err != nil {
        return nil, err
    } else {
        l := make([]interface{}, len(pl.Items))
        for i, p := range pl.Items {
            l[i] = p
        }
        return l, nil
    }
}

func (pr PodResources) Watch(selector klabels.Selector) (kwatch.Interface, error) {
    return pr.pods.Watch(selector, kfields.Everything(), "")
}