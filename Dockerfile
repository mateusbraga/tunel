FROM fedora
MAINTAINER Mateus Braga mateus.a.braga@gmail.com

RUN yum update -y
RUN yum install -y golang
ENV GOPATH /gopath
ENV PATH $PATH:$GOPATH/bin

ADD . /gopath/src/github.com/mateusbraga/tunel
WORKDIR /gopath/src/github.com/mateusbraga/tunel
RUN go install ...

EXPOSE 4000
ENTRYPOINT ["tuneld", "-bind", ":4000"]



