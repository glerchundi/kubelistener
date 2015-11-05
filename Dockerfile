FROM busybox
ADD bin/kubelistener-linux-amd64 /kubelistener
ADD src/github.com/glerchundi/kubelistener/kubelistener.go /kubelistener.go
ENTRYPOINT [ "/kubelistener" ]
