# Ephemeral Namespace Operator Local Development

### Purpose
The purpose of this doc is to describe the steps to deploy the Ephemeral Namespace Operator (ENO) in Minikube for local development. 

### Prerequisite setup steps
The following steps are required in order to successfully run the ENO in a minikube environment. 
- Clone the [Clowder](https://github.com/RedHatInsights/clowder) repository
- Install [Minikube](https://minikube.sigs.k8s.io/docs/start/)
- Login credentials for [Quay](https://quay.io)
- Clone the [Ephemeral Namespace Operator](https://github.com/RedHatInsights/ephemeral-namespace-operator) repository

### Setup Minikube
Minikube acts as the local k8s environment to run the ENO and its dependencies in. The settings below are for developing the ENO  
using pools with low requirements but they may not be sufficient if the ClowdEnvironment starts asking for more resources or if  
multiple/large pools are created.
```
minikube start --cpus 4 --disk-size 36GB --memory 8192MB --addons registry --addons ingress --addons metrics-server
```

### Retrieve Quay Pull Secret
Authentication to the private Quay repository is required for pulling container images. The following steps with walk you through  
getting your quay pull secret:
- Login to [Quay](https://quay.io)
- Click on your username at the top right of the homepage and click `Account Settings`
- At the `CLI Password` section, click `Generate Encrypted Password`
- Click `Kubernetes Secret`
- Click `Download <your username>-secret.yml`

### Setup Ephemeral Base Namespace
The `ephemeral-base` namespace is used for copying secrets to the ephemeral namespaces that are created using the ENO.

Create the `ephemeral-base` namespace.
```
oc create ns ephemeral-base --dry-run=client -o yaml | oc apply -f -
```

Apply the pull secret yaml file you retrieved from Quay.
```
oc apply -f ~/path/to/<your username>-secret.yml -n ephemeral-base
```

### Install Clowder
Change to the Clowder directory that you cloned earlier and run the following command.
```
./build/kube_setup.sh && make deploy-minikube-quick
```

### Run the Ephemeral Namespace Operator
Change to the ENO directory and apply the following CRD.
```
oc apply -f config/crd/static/cloud.redhat.com_frontendenvironments.yaml
```

The following command handles the following:
- Updates the version if necessary in the [version.txt](https://github.com/RedHatInsights/ephemeral-namespace-operator/blob/main/controllers/cloud.redhat.com/version.txt) file
- Generatea WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
- Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
- Install CRDs into the K8s cluster specified in ~/.kube/config
- Run [go fmt](https://go.dev/blog/gofmt) against code
- Run [go vet](https://pkg.go.dev/cmd/vet) against code
- Starts the ENO
```
make run
```
  
You will need to use multiple terminal windows.

### Apply Ephemeral Namespace Operator Pool Spec
In the root directory of the Ephemeral Namespace Operator there is a pool yaml file for testing called `test-pool-spec.yaml`.  
The purpose of the pool spec is the set the desired state of the namespace by spinning up provider pods based on the specifications.
This default pool spec defines namespaces with most providers set to `none` or with low requirements for the purposes of testing  
the ENO particularly.
  
Update the `name` under the `pullSecret` attribute in the `test-pool-spec.yaml` file to the name of the pull secret retrieved from Quay:
```
pullSecrets:
- namespace: ephemeral-base
  name: "EDIT THIS LINE - add the name of your quay pull secret"
```
  
Apply the file to your Minikube cluster. The ENO will reconcile and begin to spin up namespaces that have the defined specification for  
the namespaces in the pool.
```
oc apply -f ~/path/to/test-pool-spec.yaml
```

Editing the `size` attribute will define the quantity of namespaces created for a given pool.
```
spec:
  size: 2
```

If the need arises to have namespaces with differing specifications, you can define multiple pool specs. This will spin up the quantity of namespaces  
per pool from the `size` attribute.

### Check For Ephemeral Namespaces
Run the following command to check that the namespaces exist.
```
kubectl get namespaces -Lpool | grep ephemeral- | grep -v -e -base 
```

You should see the following output:
```
ephemeral-<rand 6 chars>        Active   <time>   default
ephemeral-<rand 6 chars>        Active   <time>   default
```
