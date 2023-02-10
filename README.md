# cluster-secret-operator
The cluster-secret-operator is an operator that does secrets copy/synchronization over the namespaces.
It can create secrets based on the ClusterSecret definition or copy/synchronize secrets that already exists.

## Description
This operator was created using the [Operator SDK](https://sdk.operatorframework.io) so, you might be familiar with its structure already.
The main use cases for using this operator are when you need the same secret in different namespaces.
Below we are going to explain each of the use cases.

### Secrets replication
This is for when you need to just replicate some secrets data across namespaces (most basic usage of the operator).

Below there is an example of how to replicate a basic auth secret to all namespaces:
```yaml
apiVersion: chistadata.io/v1
kind: ClusterSecret
metadata:
  name: replicated-secret
spec:
  includeNamespaces:
    - "*"
  data:
    username: SUxvdmVZb3U=
    password: RG9udFRyeVRoaXNBdEhvbWU=
  type: kubernetes.io/basic-auth
```

### Secrets copy
The secrets copy you would use when you already have a secret and want to copy it to other namespaces.

Below an example of how to use the operator to copy an existing secret to all namespaces that ends with system:
```yaml
apiVersion: chistadata.io/v1
kind: ClusterSecret
metadata:
  name: secret-copy
spec:
  includeNamespaces:
    - "system*"
  valueFrom:
    secretNamespace: kube-system
    secretName: secret-to-be-copied
---
apiVersion: v1
kind: Secret
metadata:
  namespace: kube-system
  name: secret-to-be-copied
stringData:
  flux.yaml: |
    testProperty: null-value
  third-party.yaml: |
    tryAnotherProperty: value
```

### Secrets synchronization
The synchronization is for when you want to do not only the copy from an existing secret but also keep them in sync.

To enable this synchronization, the only thing you need to do is to add the following annotation to the existing secret:

```yaml
chistadata.io/name: <name-of-the-cluster-secret>
```

## Namespace selection
In order to select which namespaces the operator will create the secrets for you,
you can use two properties: `includeNamespaces` and `excludeNamespaces`. 
The `includeNamespaces` is evaluated before the `excludeNamespaces` and by default no namespace is selected,
meaning that the `includeNamespaces` is required for you to select at least one namespace and then from this selection 
you can exclude some namespaces using the `excludeNamespace`.

You can also use the wildcard "*" in your selections to define namespaces that starts with or ends with certain strings.

NOTE: You can not use the wildcard in the middle of selection string.

### Examples
1. Select all namespaces:
```yaml
includeNamespaces:
- "*"
```
2. Select all namespaces, except the "kube-system":
```yaml
includeNamespaces:
- "*"
excludeNamespaces:
- "kube-system"
```
3. Select all namespaces, except the ones ending with "system":
```yaml
includeNamespaces:
- "*"
excludeNamespaces:
- "*system"
```
4. Select only namespaces that starts with "kube"
```yaml
includeNamespaces:
- "kube*"
```

// TODO(user): write better/custom documentation for installation and maintenance
## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:
	
```sh
make docker-build docker-push IMG=<some-registry>/cluster-secrets:tag
```
	
3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/cluster-secrets:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster 

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
