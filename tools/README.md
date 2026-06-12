# tools/

Separate Go module for development tool dependencies (linters, code generators, test runners).

Tools are invoked using:

```
go tool -modfile=tools/go.mod <tool-package>
```
