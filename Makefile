DEB_IMAGE   = unipack-test-deb
RPM_IMAGE   = unipack-test-rpm
SETUP_IMAGE = unipack-test-setup

.PHONY: test-deb test-rpm test-setup test clean

## Build dan jalankan test Debian (.deb + Alien deb→rpm)
test-deb:
	podman build --target test-deb -t $(DEB_IMAGE) -f Containerfile.test .
	podman run --rm $(DEB_IMAGE)

## Build dan jalankan test Fedora (.rpm + Alien rpm→deb)
test-rpm:
	podman build --target test-rpm -t $(RPM_IMAGE) -f Containerfile.test .
	podman run --rm $(RPM_IMAGE)

## Build dan jalankan test setup auto-install (Debian bersih)
test-setup:
	podman build --target test-setup -t $(SETUP_IMAGE) -f Containerfile.test .

## Jalankan semua test secara sequential
test: test-deb test-rpm test-setup

## Hapus image
clean:
	podman rmi -f $(DEB_IMAGE) $(RPM_IMAGE) $(SETUP_IMAGE) 2>/dev/null || true
