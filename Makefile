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
	${BIN_DIR}/deepcopy-gen -i ./pkg/batchd/apis/v1/ -O zz_generated.deepcopy

clean:
	rm -rf _output/
