# Generating example gRPC TLS certificates

## Outcomes

The outcome of this is that we will have three Secrets available in the `heptio-contour` namespace:
- cacert: contains the CA's public certificate.
- contourcert: contains Contour's keypair, used for serving TLS secured gRPC. This must be a valid certificate for the name `contour` in order for this to work. This is currently hardcoded by Contour.
- envoycert: contains Envoy's keypair, used as a client for connecting to Contour.

For the purposes of this documentation, we'll be doing the same thing the `contour certgen --pem` or the `make gencerts` commands do and putting the certs in a `certs/` subdirectory of the downloaded repo.

## Caveats and warnings

**Don't use this to create your production certificate infrastructure!**
This is intended as an example to help you get started. For any real deployment, you should **carefullY** manage all the certificates and control who has access to them. Make sure you don't commit them to any git repos either.

## Generating a CA keypair

First, we need to generate a keypair:
```
openssl req -x509 -new -nodes \
    -keyout certs/cakey.pem -sha256 \
    -days 1825 -out certs/cacert.pem \
    -subj "/O=Project Contour/CN=Contour CA"
```

Then, the new CA key will be stored in `certs/cakey.pem` and the cert in `certs/cacert.pem`.

## Generating Contour's keypair

Then, we need to generate a keypair for Contour. First, we make a new private key:
```
openssl genrsa -out certs/contourkey.pem 2048
```

Then, we create a CSR and have our CA sign the CSR and issue a cert. This uses the file [_integration/cert-contour.ext](./_integration/cert-contour.ext), which ensures that at least one of the valid names of the certificate is the bareword `contour`. This is required for the handshake to succeed, as `contour bootstrap` configures Envoy to pass this as the SNI for the connection.

```
openssl req -new -key certs/contourkey.pem \
	-out certs/contour.csr \
	-subj "/O=Project Contour/CN=contour"
openssl x509 -req -in certs/contour.csr \
    -CA certs/cacert.pem \
    -CAkey certs/cakey.pem \
    -CAcreateserial \
    -out certs/contourcert.pem \
    -days 1825 -sha256 \
    -extfile _integration/cert-contour.ext
```

At this point, the contour cert and key are in the files `certs/contourcert.pem` and `certs/contourkey.pem` respectively.

## Generating Envoy's keypair

Next, we generate a keypair for Envoy:
```
openssl genrsa -out certs/envoykey.pem 2048
```

Then, we generated a CSR and have the CA sign it:
```
openssl req -new -key certs/envoykey.pem \
	-out certs/envoy.csr \
	-subj "/O=Project Contour/CN=envoy"
openssl x509 -req -in certs/envoy.csr \
    -CA certs/cacert.pem \
    -CAkey certs/cakey.pem \
    -CAcreateserial \
    -out certs/envoycert.pem \
    -days 1825 -sha256 \
    -extfile _integration/cert-envoy.ext
```

Like the contour cert, this CSR uses the file [_integration/cert-envoy.ext](./_integration/cert-envoy.ext). However, in this case, there are no special names required.

## Putting the certs in the cluster

Next, we create the required secrets in the target Kubernetes cluster:

```
kubectl create secret -n heptio-contour generic cacert --from-file=./certs/cacert.pem
kubectl create secret -n heptio-contour tls contourcert --key=./certs/contourkey.pem --cert=./certs/contourcert.pem
kubectl create secret -n heptio-contour tls envoycert --key=./certs/envoykey.pem --cert=./certs/envoycert.pem
```

Note that we don't put the CA **key** into the cluster, there's no reason for that to be there, and that would create a security problem. That also means that the `cacert` secret can't be a `tls` type secret, as they must be a keypair.

# Conclusion

Once this process is done, doing a 
```
kubectl apply -f examples/ds-hostnet-split/
```

Will successfully apply the pods. Otherwise, the pods will not be able to be created, since the required Secrets will not be present.
