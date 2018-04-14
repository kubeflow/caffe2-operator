# caffe2-operator
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
  backend: "redis"
  replicaSpecs:
    - replicas: 1
      ReplicaType: MASTER
      template:
        spec:
          containers:
            - image: carmark/caffe2:latest
              name: caffe2
              securityContext:
                capabilities:
                  add: ["ALL"]
          restartPolicy: Never
    - replicas: 2
      ReplicaType: WORKER
      template:
        spec:
          containers:
            - image: carmark/caffe2:latest
              securityContext:
                capabilities:
                  add: ["ALL"]
              name: caffe2
          restartPolicy: Never
```

This Caffe2Job resembles the existing TFJob for the tf-operator.  The main differences being the omission of the parameter server replica type, and the addition of `backend` options.

`backend` Defines the distributed type the Caffe2 master and workers will use to communicate when initializing the worker group. Information on the different backends (and the functions they support) can be found [here](https://caffe2.ai/docs/distributed-training.html).

For `redis` backend, we will add another `redis` container into MASTER pod to be used between master and worker nodes.  

### Resulting Master

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: caffe2-master-${job_id}
  labels:
    app=caffe2-job-xx
    caffe2_job_name=example-job
    controller-uid=dc3669c6-29f1-11e8-9ccd-ac1f6b8040c6
    job-name=example-job-master-20lm-1
    job_type=MASTER
    kubeflow.org=
    runtime_id=20lm
    task_index=0
spec:
  containers:
  - image: carmark/caffe2:latest
    imagePullPolicy: IfNotPresent
    name: caffe2
  restartPolicy: Never
```

The labels variables provided are used when initializing a distributed process group with Caffe2. `task_index` is determined by adding the number of replicas in each 'MASTER' and 'WORKER' replicaSpecs. `job_type` is `MASTER` for the master.

### Resulting Worker
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: caffe2-worker-${job_id}
  labels:
    app=caffe2-job-xx
    caffe2_job_name=example-job
    controller-uid=dc3669c6-29f1-11e8-9ccd-ac1f6b8040c6
    job-name=example-job-worker-20lm-0
    job_type=WORKER
    kubeflow.org=
    runtime_id=20lm
    task_index=0
spec:
  containers:
  - image: carmark/caffe2:latest
    imagePullPolicy: IfNotPresent
    name: caffe2
  restartPolicy: Never
```

The worker spec generates a pod. They will communicate to the master through the redis's service name.

## Design
This is an implementaion of the Caffe2 distributed design patterns, found [here](https://caffe2.ai/docs/SynchronousSGD.html).

## Other backends

Form [here](https://caffe2.ai/docs/distributed-training.html), Caffe2 also support [Gloo](https://github.com/facebookincubator/gloo) which is another communications library for multi-machine training.  For Gloo with MPI, we do not neet the redis to communicate, the master and workers will communicate by ssh.  So it should be better to define another sshd port to communicate in container, then start the works first and then master container.

>Note: We do not support the `Gloo` backend. 
