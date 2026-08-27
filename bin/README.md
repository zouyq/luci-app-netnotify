# bin/

Optional prebuilt `netnotifyd` for OpenWrt packaging when the build host cannot cross-compile Go.

CI / Release artifacts are named `netnotifyd-linux-*`. To use with the Makefile, copy the matching arch binary to:

```text
bin/netnotifyd
```

Do not commit large binaries or `.ipk` files here.
