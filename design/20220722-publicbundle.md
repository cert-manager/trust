# Design: Public Trust Bundles

- [Release Signoff Checklist](#release-signoff-checklist)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [Bundle Container Server](#bundle-container-server)
  - [Behaviour After Consumption](#behaviour-after-consumption)
  - [Test Plan](#test-plan)
  - [Graduation Criteria](#graduation-criteria)
  - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
  - [Supported Versions](#supported-versions)
- [Production Readiness](#production-readiness)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)

## Release Signoff Checklist

This checklist contains actions which must be completed before a PR implementing this design can be
merged.

- [ ] This design doc has been discussed and approved
- [ ] Test plan has been agreed upon and the tests implemented
- [ ] Feature gate status has been agreed upon (whether the new functionality will be placed behind a feature gate or not)
- [ ] Graduation criteria is in place if required (if the new functionality is placed behind a feature gate, how will it graduate between stages)
- [ ] User-facing documentation has been PR-ed against the release branch in [cert-manager/website]

## Summary

Most programs, programming languages and libraries consume their trust bundles from a standard
location on-disk. On Linux, the standard bundle is typically provided by the distribution; the same
applies to containers, in which the base image used is often the provider of TLS trust bundle(s).

Currently, `trust` makes it easy to construct a bundle in a mechanical sense, but doesn't really
help in choosing / collecting the various parts which actually go into a bundle.

We propose that we should start helping in what is likely the most common case: to add an option
for a "public" trust bundle as a source.

## Motivation

NB: It's worth taking a look at the sources for Debian's [ca-certificates package](https://salsa.debian.org/debian/ca-certificates/-/tree/master/)
and [Fedora's equivalent](https://src.fedoraproject.org/rpms/ca-certificates), as background on
how popular Linux distributions package their default certificate bundles.

The ideal situation for the vast majority of applications is likely to be that they don't have to
explicitly configure their application's trust bundle.

By way of example, a Linux Golang application would [currently](https://github.com/golang/go/blob/2d655fb15a50036547a6bf8f77608aab9fb31368/src/crypto/x509/root_linux.go#L8-L15)
look in the following locations by default:

```go
var certFiles = []string{
	"/etc/ssl/certs/ca-certificates.crt",                // Debian/Ubuntu/Gentoo etc.
	"/etc/pki/tls/certs/ca-bundle.crt",                  // Fedora/RHEL 6
	"/etc/ssl/ca-bundle.pem",                            // OpenSUSE
	"/etc/pki/tls/cacert.pem",                           // OpenELEC
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // CentOS/RHEL 7
	"/etc/ssl/cert.pem",                                 // Alpine Linux
}
```

It's a chore to manually specify a location for a trust store, and it's unfamiliar and error-prone
for many users. However, if a user instead overwrites any of these files with their own
`trust`-generated certificate trust bundle they'll currently overwrite their public trust bundle
provided by their OS / container base image.

Unless the user's application only talks to services which the user also controls, what users are
likely to really want in most cases is a bundle containing both the publicly trusted certs _and_
the user's own roots.

It's possible in most distros to append to the cert bundle, but that usually involves running some
command to generate the final bundle, which would change how the user invokes their application
inside their container.

Ideally, the whole bundle a user requires would simply be available at one of the above locations.

### Goals

- A user configuring a `Bundle` can add a public trust bundle at least as easily as they can add certs from a `ConfigMap` or `Secret`
- It's clear to users exactly what version of the public bundle is used at any given time, for auditing purposes
- A user whose application must use both public and private CAs can confidently overwrite their "system" trust store in a container using a trust `Bundle`
- A user can update any bundle in use with any version of the trust controller and be confident that it will work

### Non-Goals

- Editing/pruning the public trust bundle (out of scope for the initial feature but might be desirable later)
- Supporting [OS-specific](https://superuser.com/questions/437330/how-do-you-add-a-certificate-authority-ca-to-ubuntu) certificate update mechanisms
- Supporting a plethora of options for sources of public trust bundles; we might want this in the future but we should start with one

## Proposal

We propose that the cert-manager project becomes a distributor of its own public trust bundle, which at
least to start will be a carbon copy of the Mozilla trust bundle.

That bundle in will in turn be packaged into a container which will be used as a sidecar in the trust
controller's pod, with the ability to present its own bundle to the controller container.

A deployed instance of trust will therefore contain two containers in a pod; the trust controller itself, and a bundle
container with which trust will communicate in order to fetch a trust bundle.

We'll also add a `publicBundle` field as a source, which is the method by which users include the bundle from the bundle container.

```yaml
apiVersion: trust.cert-manager.io/v1alpha1
kind: Bundle
metadata:
  name: my-org.com
spec:
  sources:
  - configMap:
      name: "my-root-cm"
      key: "root-cert.pem"
  - publicBundle:
      source: "cert-manager" # i.e. use the cert-manager bundle
  target:
    ...
```

### User Stories

#### Story 1: Standard Platform Deployment

Alice is a platform engineer using trust to ensure that the same trust bundle is present across all
of the pods in her cluster.

She upgrades to a new version of trust which supports the bundle container. The Helm chart points
to a default version of the bundle which was set at the time the version of trust was released, and
she's happy with the defaults.

After rolling out the upgrade she edits her `Bundle` resources to add a `publicBundle`. She kicks off
a fresh deployment of all pods and now they all have all the certificates she needs to talk to both
public services and services using her organization's private CA.

#### Story 2: Security Incident

Bob is a security engineer responding to a major security incident involving a root certificate
whose private key was exposed on the internet.

For each cluster in which `trust` is deployed Bob updates the Helm `bundleImage.Repository` variable
to point to the latest version of the bundle container. He then proceeds to roll out the bundle much
as Alice did above - by rolling the deployments of each service. The vulnerability is patched safely.

#### Story 3: Without the Bundle

Charles is a security engineer responding to the same incident but without already using trust inside
his clusters.

He has to update the base image of every container the organisation builds to include the new root, but
that task requires several steps:

1. Ensuring he has a list of every base image used across the organization. If he misses a container,
   it'll be vulnerable to MitM attacks until it's updated.
2. Making sure that each distinct base image used across the organization (Alpine / Debian / Distroless / etc)
   has actually received the updated trust store. There's no point updating if there's nothing to update to!
3. Rebuilding every single container which has an embedded trust bundle across the organization. In practice,
   this is nearly all of them.
4. Testing the newly built containers in CI to ensure that the updated base images haven't changed any other
   functionality. If some containers were running an out-of-support base image, they won't get the updated
   ca-certificates package, and Charles might need to bump to a new minor version which could be incompatible
   with his applications.
5. After rebuilding, he has to roll all deployments of all pods to make sure they're using the new images.

All of the above would have to happen under extreme pressure.

### Risks and Mitigations

#### Bundle Container Updates

This proposal makes it absolutely critical that we have a smooth and rapid release process for the
"bundle" container. A security incident involving a root certificate would be front-page news and a
major incident for basically every organisation in the world and we'd have to treat it the same,
to the point of waking engineers up in the middle of the night to cut a new release if required.

#### Bundle Container Attack Surface

Since this proposal includes a need for some kind of process by which the controller container can
query the bundle from the bundle container, that necessitates an increase in the attack surface of
trust as an overall application.

The communication channel between the controller and bundle containers is absolutely security critical;
if a malicious actor were able to intercept and modify the bundle in-flight it would allow them to insert
their own root certificate and MitM every connection between any container using the resulting bundle.

This is mitigated by the use of two containers in a Pod - the Kubernetes security model should allow
for this to be done securely without interference.

## Design Details

### Bundle Container "Server"

The initial version of the bundle container would simply embed the trust bundle into a small Go
application, similar to what [this PR](https://github.com/cert-manager/trust/pull/36) does.

This Go application would then expose the certificate to the controller pod by writing the bundle,
to a known location on [a shared volume](https://kubernetes.io/docs/tasks/access-application-cluster/communicate-containers-same-pod-shared-volume/).

While it would likely be sufficient to write a PEM-encoded trust bundle directly onto disk, we can
future-proof a little by wrapping the bundle into a JSON blob. This way, we can choose to add more
fields to the JSON blob and be sure to ignore them in the trust controller. As long as we don't
remove or change the type of existing fields, the same blob should be usable with any client.

The initial blob might look like this:

```json
{
	"pemBundle": "<pem-encoded-trust-bundle",
	"bundleVersion": "some version number",
	"bundleType": "static|dynamic"
}
```

`pemBundle` is the bundle itself in PEM format.

`bundleVersion` is the version of the bundle application which served the bundle, which might be
useful for logging.

`bundleType` would indicate whether the bundle is expected to ever change or whether it should
be rechecked periodically by the trust controller.

Other fields might be required but the number should be kept to a minimum since each adds a
maintenance and testing burden. Again, it's key that every version of the bundle container is
interoperable with every version of the controller container.

### Behaviour After Consumption

The reasoning for a `bundleType` field would be to enable future support of bundles which might
change dynamically, while preserving the notion of a "static" bundle which doesn't change.

A bundle marked as "static" is desirable since if the bundle won't change after it has been consumed,
the bundle container might reasonably exit with a success status code.

This prevents it from consuming any resources after its job is complete, and prevents the need for
the trust controller container to have a job to fetch updates which will never arrive.

This "stop-after-consumption" behaviour would be difficult if we later wanted to a create a version
of the bundle container which automatically updates the bundle, however - hence the need for "dynamic"
bundles as well.

The default bundle type would have to be dynamic, since that behaviour is more complex and would
require on-the-fly changes to published bundle targets. However, the initial version of the bundle
container we publish would likely be static, since that's the most common use case we'd expect.

### Test Plan

End-to-end tests should be written which use public bundles in a trust object, and ensure that the
bundle is correctly inserted into the output.

In addition, an end-to-end test should be created to ensure the "serve-and-shutdown" behaviour of
the bundle container.

### Upgrade / Downgrade Strategy

For an implementation of this design to be acceptable, it should ensure that it's _always_ trivial to
upgrade and downgrade between versions of the bundle container. That implies that the API between
the bundle container and trust container should be absolutely ironclad with regards to backwards
compatibility.

## Alternatives

### Bundling With Trust

The obvious alternative here is that the public trust bundle be embedded into trust itself and not
separated out. This makes the architecture simpler, but might introduce issues when it comes to
upgrading and downgrading.

This assumes we'll presumably end up in a similar situation with trust as we have in cert-manager;
we support a subset of available `trust` versions with updates while deprecating others. So we'd
support `v1.2` and `v1.3` but all previous versions are deprecated.

However, if there's a compromise of a publicly trusted root, it's likely to be one of the most
consequential computer security incidents ever and everyone will scramble to update immediately.

That scramble should be made as easy as possible. It's likely not feasible for us to update trust
bundles baked into every release of trust itself, so it's easier to separate that upgrade path out.

Having the public bundle be a separate component also helps us to develop mechanisms by which it
can be easily updated. For example, we could add an auto-updater version of the bundle container
which would pull an updated version automatically, allowing cluster operators to choose to opt-in
to automatic updates if they so desire.

### Just Doing Nothing

See "Motivation" above. There is a _very_ clear need for this kind of functionality.
