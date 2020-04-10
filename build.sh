#/bin/bash
args="$2"
[[ "$1" == "external" ]] && make "$2" -f Makefile.external || echo "nothing to do: $1, $2"
