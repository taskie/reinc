# reinc

include files using RegExp

## Usage

```
reinc -P '(?m)^@(.+?)@\n' -o out.txt in.txt
```

or

```
reinc -P '(?m)^@(.+?)@\n' - <in.txt >out.txt
```

### Input file (in.txt)

```txt
---
@parts/index.txt@
---
```

### Included file (parts/index.txt)

```txt
@hello.txt@
```

### Included file (parts/hello.txt)

```txt
Hello, world!
```

### Output file (out.txt)

```txt
---
Hello, world!
---
```

## License

Apache License 2.0
