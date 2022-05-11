# highness-network-controller
A custom Kubernetes controller for Network

## Running

```shell
$ go mod tidy
$ go build -o highness-network-controller .
$ ./highness-network-controller -kubeconfig=$HOME/.kube/config -alsologtostderr=true
```

## Usage

You should create the CRD of Network first:

```shell
$ kubectl apply -f crd/network.yaml
```

You can then trigger an event by creating a Network API instance:

```shell
$ kubectl apply -f example/example-network.yaml
```

CURD the Network API instance, and check the logs of controller. 
