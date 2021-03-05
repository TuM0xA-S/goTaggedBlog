FROM golang

WORKDIR /go/src/goTaggedBlog
COPY . .

RUN go build .

CMD ./goTaggedBlog
