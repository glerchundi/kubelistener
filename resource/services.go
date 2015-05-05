package resource

import (
    kclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
    kfields "github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
    klabels "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
    kwatch "github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
)

type ServiceResources struct {
    services kclient.ServiceInterface
}

func NewServiceResources(services kclient.ServiceInterface) ServiceResources {
    return ServiceResources{services}
}

func (sr ServiceResources) List(selector klabels.Selector) ([]interface{}, error) {
    if sl, err := sr.services.List(selector); err != nil {
        return nil, err
    } else {
        l := make([]interface{}, len(sl.Items))
        for i, s := range sl.Items {
            l[i] = s
        }
        return l, nil
    }
}

func (sr ServiceResources) Watch(selector klabels.Selector) (kwatch.Interface, error) {
    return sr.services.Watch(selector, kfields.Everything(), "")
}