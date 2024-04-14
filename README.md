# operator

## Annotations

```yaml
## ingress annotations
# exposes this ingress to the public internet.
# the domain will be registered with external-dns and exposed using the external ingress class.
# defaults to "false".
ingress.infra.d464.sh/public: "true"
# forces ssl connections on this ingress.
# defaults to "false".
ingress.infra.d464.sh/force-ssl: "true"
# if the ingress is public then proxy it using cloudflare.
# otherwise a cname is created that points the domain to our public ip.
# defaults to "false".
ingress.infra.d464.sh/proxied: "true"

## service annotations
# domain name that will point to this service's ip address.
# this domain is only available in the local network.
# if left empty, no domain is created.
# this is only valid on load balancer services.
# defaults to "".
service.infra.d464.sh/domain: "mydomain.d64.sh"
# expose this port to the public.
# this will create a port foward on the router to this service's ports.
# this is only valid on load balancer services.
# defaults to "false".
service.infra.d464.sh/expose: "true"
```

## CRDs

### MinioServiceAccount

This resource represents a service account in minio.
It will create a secret with the credentials used to access minio.

```yaml
apiVersion: infra.d464.sh/v1
kind: MinioServiceAccount
metadata:
  name: mads
spec:
  buckets: [mads]
```

The generate secret will look like:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mads-minio
data:
  S3_URL: http://minio.storage.svc.cluster.local:9000
  S3_ACCESS_KEY: <access key>
  S3_SECRET_KEY: <secret key>
```

### Postgres

This resource represents a postgres instance.
It will create a deployment, service and secret.

```yaml
apiVersion: infra.d464.sh/v1
kind: Postgres
metadata:
  name: mads
spec:
  tag: "16"
  replicas: 1             # this value is optional and can be 1 or 0
  resources:
    requests:
      memory: "128Mi"
      cpu: "100m"
    limits:
      memory: "512Mi"
      cpu: "2"
  storage:
    size: 1Gi
    storageClassName: blackmesa-local
```

The secret will look like:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mads-pg
data:
  DATABASE_HOST: "mads-pg"
  DATABASE_PORT: "5432"
  DATABASE_USER: "user"
  DATABASE_PASS: "pass"
  DATABASE_NAME: "dbname"
  DATABASE_URL: "postgres://user:pass@mads-pg:5432/dbname"
```

### PortForward

This resource represents a port foward on the router.

```yaml
apiVersion: infra.d464.sh/v1
kind: PortForward
metadata:
  name: my-port-forward
spec:
  address: 10.0.3.5
  port: 80
  externalPort: 80
  protocol: "tcp"             # "tcp" or "udp"
```

### DomainName

This resource represents a domain name on the local network.

```yaml
apiVersion: infra.d464.sh/v1
kind: DomainName
metadata:
  name: my-domain-name
spec:
  domain: git.d464.sh
  address: 10.0.3.5
```

## webapi

The operator exposes a simple http web api on port 3000.
```
GET /hosts        -> hosts file containing the DomainNames on the cluster.
GET /forward      -> text file containing the PortForwards on the server.
```
These endpoinds are used by the router to update dns and open ports.

## limitations

+ Only the first domain used in an ingress is exposed on the local network.
+ Only the first ip address of a load balancer service is considered.

## Developing

### Prerequisites
- go version v1.20.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

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

