# Managed binary slots

`embedded/pack.sh` кладет `zakopd` binaries в arch directories:

- `linux-amd64/`
- `linux-arm64/`
- `linux-armv7/`
- `linux-mips-softfloat/`
- `linux-mipsle-softfloat/`

Managed `sing-box` binaries можно положить в эти же directories перед packing.
Они попадут в embedded archive и будут установлены в:

```text
/usr/libexec/zakop/sing-box
```

Managed `sing-box` используется только если system `sing-box` отсутствует или
несовместим. `/usr/bin/sing-box` никогда не перезаписывается.
