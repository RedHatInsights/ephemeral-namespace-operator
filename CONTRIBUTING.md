# Ephemeral Namespace Operator Local Development

### Purpose
The purpose of this doc is to deploy the Ephemeral Namespace Operator (ENO) in Minikube for local development.  

### Requirements
- Clone the [Clowder](https://github.com/RedHatInsights/clowder) repository
- Install [Minikube](https://minikube.sigs.k8s.io/docs/start/)
- Login credentials for [Quay](https://quay.io)
- Install [Bonfire](https://pypi.org/project/crc-bonfire/)

### Setup Minikube
```
minikube start --cpus 4 --disk-size 36GB --memory 8192MB --addons registry --addons ingress --addons metrics-server
```

### Retrieve Quay Pull Secret
The following steps with walk you through getting your quay pull secret:
- Login to [Quay](https://quay.io)
- Click on your username at the top right of the homepage and click `Account Settings`
- At the `CLI Password` section, click `Generate Encrypted Password`
- Click `Kubernetes Secret`
- Click `Download <your username>-secret.yml`

### Setup Ephemeral Base Namespace
The `ephemeral-base` namespace is used for copying secrets to the ephemeral namespaces that are created using the ENO.

Run the following command to create the namespace:
```
oc create ns ephemeral-base --dry-run=client -o yaml | oc apply -f -
```

Apply the pull secret yaml file you retrieved from Quay:
```
oc apply -f ~/path/to/<your username>-secret.yml -n ephemeral-base
```

### Install Clowder
Change to the Clowder directory and run:
```
./build/kube_setup.sh && make deploy-minikube-quick
```

### Run the Ephemeral Namespace Operator
Change to the ENO directory and apply the following CRD:
```
oc apply -f config/crd/static/cloud.redhat.com_frontendenvironments.yaml
```

Start the ENO:
```
make run
```

You will need to use multiple terminal windows.

### Apply Ephemeral Namespace Operator Pool Spec
In the root directory of the Ephemeral Namespace Operator there is a pool yaml file for testing called `test-pool-spec.yaml`.  
Ensure to edit line 56 of the file to update the name of the pull secret to the one retrieved from Quay. You will want to download that and  
apply the file to your Minikube cluster with the following command:  
```
oc apply -f ~/path/to/test-pool-spec.yaml
```

You can have multiple files with different configurations which will create various pool objects. This will spin up the number  
of namespaces defined in the spec per pool. Make sure to change the name of the pool spec in the file and if you want to edit the  
namespace quantity update the `size` attribute.  

### Check For Ephemeral Namespaces
The following command will list the namespaces and their status:
```
bonfire namespace list
```

You should see the following output:
```
NAME              RESERVED    ENV STATUS    APPS READY    REQUESTER    POOL TYPE    EXPIRES IN
----------------  ----------  ------------  ------------  -----------  -----------  ------------
ephemeral-<random 6 chars>  false       ready         none                       default      TBD
ephemeral-<random 6 chars>  false       ready         none                       default      TBD
```
