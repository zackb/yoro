#!/bin/bash

# Yoro Release Automation Script
# Usage: ./scripts/release.sh <version>
# Example: ./scripts/release.sh 0.1.0

set -e

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version> (e.g., 0.1.0)"
    exit 1
fi

# Normalize: accept either "0.1.0" or "v0.1.0", work with the bare version.
VERSION="${VERSION#v}"
TAG="v$VERSION"
REPO_ROOT=$(git rev-parse --show-toplevel)
AUR_GIT_DIR="$REPO_ROOT/../yoro-git"
AUR_BIN_DIR="$REPO_ROOT/../yoro-bin"
TARBALL="$REPO_ROOT/build/release/yoro-$VERSION-linux-amd64.tar.gz"

# 1. Validation
if ! command -v gh &> /dev/null; then
    echo "Error: 'gh' (GitHub CLI) is not installed."
    exit 1
fi

if ! git diff-index --quiet HEAD --; then
    echo "Error: You have uncommitted changes. Please commit or stash them first."
    exit 1
fi

CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo "Error: You are on branch '$CURRENT_BRANCH'. Releases must be performed from 'main'."
    exit 1
fi

echo "🚀 Starting release process for $TAG..."

# 2. Bump version in the man page and in-repo PKGBUILDs
sed -i "s/\.TH YORO 1 \"[^\"]*\" \"yoro [0-9][0-9.]*\"/.TH YORO 1 \"$(date +'%B %Y')\" \"yoro $VERSION\"/" "$REPO_ROOT/man/yoro.1"
sed -i "s/^pkgver=.*/pkgver=$VERSION/" "$REPO_ROOT/packaging/aur-git/PKGBUILD"
sed -i "s/^pkgver=.*/pkgver=$VERSION/" "$REPO_ROOT/packaging/aur-bin/PKGBUILD"
git add "$REPO_ROOT/man/yoro.1" "$REPO_ROOT/packaging/aur-git/PKGBUILD" "$REPO_ROOT/packaging/aur-bin/PKGBUILD"
git commit -m "chore: bump version to $VERSION" || true

# 3. Tag and Push
echo "🏷️  Tagging $TAG..."
if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo "Warning: Tag $TAG already exists locally."
else
    git tag -a "$TAG" -m "Release $TAG"
fi
git push origin main
git push origin "$TAG"

# 4. Build Package
echo "📦 Building package..."
rm -rf "$REPO_ROOT/build/release"
make -C "$REPO_ROOT" package VERSION="$VERSION"

# 5. Create GitHub Release
echo "🌐 Creating GitHub Release..."
gh release create "$TAG" "$TARBALL" --title "Release $TAG" --generate-notes

# 6. Update AUR (yoro-git)
if [ -d "$AUR_GIT_DIR" ]; then
    echo "🧬 Updating yoro-git AUR..."
    cp "$REPO_ROOT/packaging/aur-git/PKGBUILD" "$AUR_GIT_DIR/PKGBUILD"
    sed -i "s/^pkgver=.*/pkgver=$VERSION/" "$AUR_GIT_DIR/PKGBUILD"
    (
        cd "$AUR_GIT_DIR"
        makepkg --printsrcinfo > .SRCINFO
        git add PKGBUILD .SRCINFO
        git commit -m "update to $VERSION"
        git push
    )
    echo "   yoro-git updated and pushed."
else
    echo "⚠️  Warning: $AUR_GIT_DIR not found, skipping."
fi

# 7. Update AUR (yoro-bin)
if [ -d "$AUR_BIN_DIR" ]; then
    echo "🏗️  Updating yoro-bin AUR..."
    SHA256=$(sha256sum "$TARBALL" | cut -d' ' -f1)
    cp "$REPO_ROOT/packaging/aur-bin/PKGBUILD" "$AUR_BIN_DIR/PKGBUILD"
    sed -i "s/^pkgver=.*/pkgver=$VERSION/" "$AUR_BIN_DIR/PKGBUILD"
    sed -i "s/^sha256sums=.*/sha256sums=('$SHA256')/" "$AUR_BIN_DIR/PKGBUILD"
    (
        cd "$AUR_BIN_DIR"
        makepkg --printsrcinfo > .SRCINFO
        git add PKGBUILD .SRCINFO
        git commit -m "update to $VERSION"
        git push
    )
    echo "   yoro-bin updated and pushed."
else
    echo "⚠️  Warning: $AUR_BIN_DIR not found, skipping."
fi

echo "✅ Full release $VERSION successfully deployed to GitHub and AUR!"
