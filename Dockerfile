FROM golang:1.4-onbuild

COPY . /usr/src/app
RUN go get -d -v
RUN go install -v
