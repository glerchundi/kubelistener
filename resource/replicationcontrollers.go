package resource

import (
    kclient "github.com/GoogleCloudPlatform/kubernetes/pkg/client"
    kfields "github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
    klabels "github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
    kwatch "github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
)

type ReplicationControllerResources struct {
    replicationControllers kclient.ReplicationControllerInterface
}

func NewReplicationControllerResources(rcs kclient.ReplicationControllerInterface) ReplicationControllerResources {
    return ReplicationControllerResources{rcs}
}

func (rcr ReplicationControllerResources) List(selector klabels.Selector) ([]interface{}, error) {
    if rcl, err := rcr.replicationControllers.List(selector); err != nil {
        return nil, err
    } else {
        l := make([]interface{}, len(rcl.Items))
        for i, rc := range rcl.Items {
            l[i] = rc
        }
        return l, nil
    }
}

func (rcr ReplicationControllerResources) Watch(selector klabels.Selector) (kwatch.Interface, error) {
    return rcr.replicationControllers.Watch(selector, kfields.Everything(), "")
}