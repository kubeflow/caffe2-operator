BIN_DIR=_output/bin
GO_FLAGS = #-v

all: build

.PHONY: clean init build linux darwin

clean:
	go clean -i $(GO_FLAGS) .
	rm -rf ${BIN_DIR}

init:
	mkdir -p ${BIN_DIR}

build: init
	go build $(GO_FLAGS) -o ${BIN_DIR}/caffe2-operator ./cmd/caffe2-operator/

linux: init
	GOOS=linux GOARCH=amd64 go build $(GO_FLAGS) -o ${BIN_DIR}/caffe2-operator ./cmd/caffe2-operator/

darwin: init
	GOOS=darwin GOARCH=amd64 go build $(GO_FLAGS) -o ${BIN_DIR}/caffe2-operator ./cmd/caffe2-operator/

verify: generate-code
	hack/verify-gofmt.sh
	hack/verify-golint.sh
	hack/verify-gencode.sh

generate-code:
	hack/update-codegen.sh

