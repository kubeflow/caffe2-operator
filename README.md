## :warning: **kubeflow/caffe2-operator is not maintained**

This repository has been deprecated, and will be [archived](https://github.com/kubeflow/community/issues/479) soon (Nov 30th, 2021). 


# caffe2-operator
[![Build Status](https://travis-ci.com/kubeflow/caffe2-operator.svg?branch=master)](https://travis-ci.org/kubeflow/caffe2-operator) [![Go Report Card](https://goreportcard.com/badge/github.com/kubeflow/caffe2-operator)](https://goreportcard.com/report/github.com/kubeflow/caffe2-operator)

Experimental repository for a caffe2 operator

## Motivation
Caffe2 is a popular machine learning framework which currently does not have an operator/controller for Kubernetes. This proposal is aimed at defining what that operator should look like, and adding it to Kubeflow.

For distributed training, Caffe2 has no parameter server compared with Tensorflow, so it has to use Redis/Gloo to find the other nodes to communicate.

## Build
```
$ make
mkdir -p _output/bin
go build -o _output/bin/caffe2-operator ./cmd/caffe2-operator/
$ _output/bin/caffe2-operator --help

```

### Custom Resource Definition
The custom resource submitted to the Kubernetes API would look something like this:
```yaml
apiVersion: "kubeflow.org/v1alpha1"
kind: "Caffe2Job"
metadata:
  name: "example-job"
spec:
  backendSpecs:
      backendType: redis
      redisHost: redis-service
      redisPort: 6379
  replicaSpecs:
      replicas: 2
      template:
        spec:
          hostNetwork: true
          containers:
          - image: kubeflow/caffe2:py2-cuda9.0-cudnn7-ubuntu16.04
            name: caffe2
            resources:
              limits:
                nvidia.com/gpu: 2
            workingDir: /usr/local/caffe2/caffe2/python/examples/
            command: ["python", "resnet50_trainer.py"]
```

A full resnet50 trainer with redis example is [here](./examples/resnet50_redis.yaml), and resnet50 trainer with nfs example is [here](./examples/resnet50_nfs.yaml).

This Caffe2Job resembles the existing TFJob for the tf-operator.  The main differences being the omission of the parameter server replica type, and the addition of `backend` options.

`backend` Defines the distributed type the Caffe2 master and workers will use to communicate when initializing the worker group. Information on the different backends (and the functions they support) can be found [here](https://caffe2.ai/docs/distributed-training.html).

For `redis` backend, you need a working Redis server to serve for workers communication.

### Resulting Worker
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: caffe2-worker-${job_id}
  labels:
      caffe2_job_key: default-example-job
      caffe2_replica_index: "0"
      caffe2_replica_type: worker
      group_name: kubeflow.org
      runtime_id: "1529307087"
spec:
  containers:
    image: kubeflow/caffe2:py2-cuda9.0-cudnn7-ubuntu16.04
    imagePullPolicy: IfNotPresent
    name: caffe2
    env:
      - name: SHARD_ID
        value: "0"
      - name: NUM_SHARDS
        value: "1"
      - name: RUN_ID
        value: "1529307087"
      - name: CAFFE2_CONFIG
        value: '{"cluster":{"worker":["default-example-job-worker-0:2222"]},"task":{"type":"worker","index":0}}'
      - name: REDIS_HOST
        value: "redis-service"
        value: "1529307087"
    ...
```

The worker spec generates a pod. They will communicate to the master through the redis's service name.

>NOTE: There are three additional environments which are generated based on worker role, such as `index` for `SHARD_ID`, `replicas` for `NUM_SHARDS` and running ID for `RUN_ID`.

## Design
This is an implementaion of the Caffe2 distributed design patterns, found [here](https://caffe2.ai/docs/SynchronousSGD.html).

## Other backends

Form [here](https://caffe2.ai/docs/distributed-training.html), Caffe2 also support NFS backend, however, we do not test the `nfs` backend now. 

## How to setup

### Setup kubernetes
* A full function kubernetes.
* Open the `features-gate` if you want to use GPU

### Create a CRD for kuberntes

```
# kubectl apply -f https://raw.githubusercontent.com/kubeflow/caffe2-operator/master/examples/crd.yaml
customresourcedefinition.apiextensions.k8s.io "caffe2jobs.kubeflow.org" created
```

### Start the caffe2-operator

```
# ./caffe2-operator -alsologtostderr -v 4 -controller-config-file /root/admin.conf
```

### Prepare the dataset

In the example, we use [handwritten](http://yann.lecun.com/exdb/mnist/). You need to convert it to levelDB type by using `make_mnist_db`.
```
$ make_mnist_db --channel_first --db leveldb --image_file data/mnist/train-images-idx3-ubyte --label_file data/mnist/train-labels-idx1-ubyte --output_file data/mnist/mnist-train-nchw-leveldb 

$ make_mnist_db --channel_first --db leveldb --image_file data/mnist/t10k-images-idx3-ubyte --label_file data/mnist/t10k-labels-idx1-ubyte --output_file data/mnist/mnist-test-nchw-leveldb 
```

### Run the job

```
$ kubectl apply -f ./examples/resnet50_nfs.yaml
$ kubectl get caffe2jobs
NAME          AGE
example-job   29m
$ kubectl get pods
NAME                READY     STATUS    RESTARTS   AGE
example-job-pdcbs   1/1       Running   0          29m
example-job-abcdc   1/1       Running   0          29m
```

In this example, we use `hostNetwork = true`, it is not the better solution, but it will train more quickly. Because the overlay network will reduce some performance.
