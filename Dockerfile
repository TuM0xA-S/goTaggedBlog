FROM golang

WORKDIR /go/src/goTaggedBlog

COPY go.mod go.sum ./

RUN go mod download -x

COPY . .

RUN go build .

CMD ["./goTaggedBlog"]
