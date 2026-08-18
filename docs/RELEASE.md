# Release Process

Этот документ описывает, как выпускать новый GitHub Release для `zakop`.

Версия `zakopd` не хранится в отдельном Go файле. При сборке archive
`embedded/pack.sh` берет version из git tag через:

```sh
git describe --tags --always --dirty
```

и подставляет ее в binary через Go linker:

```sh
-ldflags "-X main.version=$VERSION"
```

Поэтому normal release flow - это git tag + archive upload.

## 1. Prepare Worktree

Проверить, что нет незакоммиченных изменений:

```sh
git status
```

Если изменения есть:

```sh
git add .
git commit -m "Prepare release vX.Y.Z"
```

## 2. Create Tag

Для release:

```sh
git tag vX.Y.Z
```

Например, для patch release после `v1.0.0`:

```sh
git tag v1.0.1
```

Проверить:

```sh
git tag --list --sort=-version:refname | head
git describe --tags --always --dirty
```

## 3. Build Archive

```sh
GOCACHE=/tmp/zakop-go-cache ./embedded/pack.sh
./scripts/test-archive.sh
```

Archive будет здесь:

```text
dist/zakop-openwrt-embedded.tar.gz
```

Проверить version внутри binary:

```sh
tmp="$(mktemp -d)"
tar -xzf dist/zakop-openwrt-embedded.tar.gz -C "$tmp"
"$tmp/zakop/bin/linux-amd64/zakopd" version
rm -rf "$tmp"
```

Expected:

```text
zakopd vX.Y.Z
```

## 4. Push To GitHub

```sh
git push origin main
git push origin vX.Y.Z
```

Или все tags:

```sh
git push origin --tags
```

## 5. Publish GitHub Release

На GitHub:

1. Open repository.
2. Go to Releases.
3. Draft a new release.
4. Select tag `vX.Y.Z`.
5. Release title: `vX.Y.Z`.
6. Upload asset:

```text
zakop-openwrt-embedded.tar.gz
```

7. Publish release.

Installer downloads archive from:

```text
https://github.com/elllkere/zakop/releases/latest/download/zakop-openwrt-embedded.tar.gz
```

Поэтому asset name должен быть точно:

```text
zakop-openwrt-embedded.tar.gz
```

## Manual Version Override

Если нужно собрать archive без git tag:

```sh
ZAKOP_VERSION=vX.Y.Z GOCACHE=/tmp/zakop-go-cache ./embedded/pack.sh
```

Но для public release лучше использовать git tag, чтобы GitHub Release,
`git describe` и `zakopd version` совпадали.

## Version Source

Текущая release version берется из Git tag на commit, который собирается.
Проверить локально:

```sh
git describe --tags --always --dirty
```
