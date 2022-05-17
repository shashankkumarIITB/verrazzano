# Verrazzano helm-based install POC

This directory tree has POCs for helm-based install and profiles. There is a top-level
`verrazzano-instance` helm chart and an `ingress` subchart.  The top-level `verrazzano-instance`
chart is used to install existing Verrazzano CR.  The `ingress` sub-chart is used to
deploy a Kubernetes Ingress resource.  Profiles are used to control the resources 
and components that get installed. The POC assumes that all operators are installed. 

## Prerequisites
1. Create a kind cluster with MetalLB
2. Install the Verrazzano platform operator from master

## POC 1 - NGINX + Ingress with app
The POC will install NGINX Ingress Controller, create an Ingress, then deploy a sample helidon application.

**1. Install NGINX and Ingress**  
Change directory to `verrazzzano/platform-operator/manifests/poc` and run the following command to install NGINX Ingress Controller and create and Ingress
```
helm install test charts/verrazzano-instance  -f profiles/ingress.yaml -f profiles/nginx.yaml 
```
Wait until Verrazzano is installed. Also, check the helm releases using `helm list -A`.
You should see both `test` and `ingress-controller` releases.

**2. Get the IP Ingress**
```
kubectl get service -n ingress-nginx ingress-controller-ingress-nginx-controller
```
Get the EXTERNAL-IP field, for example `172.18.0.230`

**3. Update the hostname in the Ingress**  
Change the host field to hello.<IP>.nip.io, for example:
```
kubectl edit ingress test

spec:
  rules:
  - host: hello.172.18.0.230.nip.io
```

**4. Deploy the application**
```
kubectl apply -f apps/hello/app.yaml
```

**5. Access the application**
```
curl -k https://hello.172.18.0.230.nip.io/greet
```
