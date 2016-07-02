FROM golang:1.7-wheezy
RUN mkdir /opt/kettle-watcher
ADD src/github.com/microKettle /opt/kettle-watcher/src/github.com/microKettle/
ENV GOPATH=/opt/kettle-watcher
ENV GOBIN=/opt/kettle-watcher/bin
WORKDIR /opt/kettle-watcher
RUN cd /opt/kettle-watcher/src/github.com/microKettle/watcher && go get . && go install
CMD ["/opt/kettle-watcher/bin/watcher"]
