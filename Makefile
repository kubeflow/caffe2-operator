BIN_DIR=_output/bin

caffe2-operator: init
	go build -o ${BIN_DIR}/caffe2-operator ./cmd/caffe2-operator/

verify: generate-code
	hack/verify-gofmt.sh
	hack/verify-golint.sh
	hack/verify-gencode.sh

init:
	mkdir -p ${BIN_DIR}

generate-code:
	hack/update-codegen.sh

clean:
	rm -rf _output/
