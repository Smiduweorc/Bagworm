# Maintainer: Smiduweorc <pendragonscode@gmail.com>
pkgname=bagworm
pkgver=0.1.0
pkgrel=1
pkgdesc="The opinionated developer UX for OCI containers - bagworm enter drops you into your project's image"
arch=("x86_64")
url="https://github.com/Smiduweorc/bagworm"
license=('MIT')
depends=()
optdepends=('podman: container runtime'
            'docker: container runtime'
            'nerdctl: container runtime')
makedepends=("go" "git")
source=("$pkgname-$pkgver.tar.gz::https://github.com/Smiduweorc/bagworm/archive/refs/tags/v$pkgver.tar.gz")
sha256sums=("SKIP")

build() {
	cd "$pkgname-$pkgver"
	go build -trimpath -ldflags="-s -w -X main.version=v$pkgver" -o bagworm ./cmd/bagworm
}

package() {
	cd "$pkgname-$pkgver"
	install -Dm755 bagworm "$pkgdir/usr/bin/bagworm"
	install -Dm644 LICENSE "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
	install -Dm644 bagworm.example.yaml "$pkgdir/usr/share/doc/$pkgname/bagworm.example.yaml"
}
