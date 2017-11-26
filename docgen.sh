PRE=$(cat readme_begin.md)
USAGE=$(./sampctl docs)
POST=$(cat readme_end.md)
printf "$PRE\n\n#$USAGE\n\n$POST" >README.md
