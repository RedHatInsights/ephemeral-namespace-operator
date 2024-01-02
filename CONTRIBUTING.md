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
Save the following as a file. YOu can have multiple files with different configurations which will create various pool objects.  
This will spin up the number of namespaces defined in the spec per pool. If you want to edit the namespace quantity, update the  
`size` attribute.  

Ensure to edit line 56 of the file to update the name of the pull secret to the one retrieved from Quay.
<details>
  <summary>Click here for NamespacePoolDefault.yaml</summary>

  ```yaml
---
apiVersion: cloud.redhat.com/v1alpha1
kind: NamespacePool
metadata:
  name: default
spec:
  size: 2
  local: True
  clowdenvironment:
    providers:
      db:
        mode: none
      inMemoryDb:
        mode: none
      kafka:
        mode: none
        enableLegacyStrimzi: true
        cluster:
          resources:
            limits:
              cpu: 500m
              memory: 1Gi
            requests:
              cpu: 250m
              memory: 600Mi
          version: 3.0.0
        connect:
          version: 3.0.0
          image: quay.io/cloudservices/xjoin-kafka-connect-strimzi:latest
          resources:
            limits:
              cpu: 500m
              memory: 1Gi    
            requests:
              cpu: 250m
              memory: 512Mi
      logging:
        mode: none
      metrics:
        port: 9000
        path: /metrics
        prometheus:
          deploy: true
        mode: none
      objectStore:
        mode: minio
      web:
        mode: operator
        ingressClass: openshift-default
        port: 8000
        privatePort: 10000
      featureFlags:
        mode: none
      pullSecrets:
      - namespace: ephemeral-base
        name: "EDIT THIS LINE - add the name of your quay pull secret"
      testing:
        k8sAccessLevel: edit
        configAccess: environment
        iqe: 
          ui: 
            selenium: 
              imageBase: quay.io/redhatqe/selenium-standalone
              defaultImageTag: ff_91.5.1esr_gecko_v0.30.0_chrome_98.0.4758.80
              resources: 
                limits: 
                  cpu: 1
                  memory: 3Gi 
                requests: 
                  cpu: 500m
                  memory: 512Mi
          vaultSecretRef: 
            namespace: ephemeral-base
            name: iqe-vault  
          imageBase: quay.io/cloudservices/iqe-tests
          resources: 
            limits: 
              cpu: 1
              memory: 2Gi          
            requests: 
              cpu: 200m
              memory: 1Gi
    resourceDefaults:
      limits:
        cpu: 300m
        memory: 256Mi
      requests:
        cpu: 30m
        memory: 128Mi
  limitrange:
    spec:
      limits:
      - type: Container
        default:
          cpu: 200m
          memory: 512Mi
        defaultRequest:
            cpu: 100m
            memory: 384Mi
      - type: Pod
        maxLimitRequestRatio:
          cpu: 10
          memory: 2
    metadata:
      name: resource-limits
  resourcequotas:
    items:
    - spec:
          hard:
            limits.cpu: 32
            limits.memory: 64Gi
            requests.cpu: 1
            requests.memory: 32Gi
          scopes: [NotTerminating]
      metadata:
        name: compute-resources-non-terminating
    - spec:
          hard:
            limits.cpu: 6
            limits.memory: 24Gi
            requests.cpu: 3
            requests.memory: 12Gi
          scopes: [Terminating]
      metadata:
        name: compute-resources-terminating
    - spec:
        hard:
          pods: 200
      metadata:
        name: pod-count
  ```
</details>
<br>
Apply the pool object. The ENO will start the reconciliation process and spin up namespaces.
```
oc apply -f ~/path/to/NamespacePooldefault.yaml
```

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
