#! /bin/bash

find / \( -name .Trash -o -type l -o -path "/Volumes/*" -o -path "/System/Volumes/*" -o -path "/Users/voss/Library/CloudStorage/*" \) -prune \
    -o -type f \( -name "*.pfa" -o -name "*.pfb" -o -name "*.afm" \) -print 2>/dev/null \
| sort \
>all-fonts

wc -l all-fonts | awk '{ print $1 " fonts found" }'

echo "done"
