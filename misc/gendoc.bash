#!/bin/bash

bin/minimega -doc > doc/markdown/api && echo -e "ok\tAPI generation" || echo -e "fail\tAPI generation"

for i in `ls doc/markdown`
do
	cat doc/template/header.html > doc/$i.html

	# special magic for a TOC for the API
	if [ $i == "api" ]
	then
		echo "<h1>minimega API</h1>" >> doc/$i.html

		echo "<div class=toc>" >> doc/$i.html
		for j in `sed -rn 's/<h2 id=([a-z_]*).*/\1/p' doc/markdown/api`
		do
			echo "<a href=\"api.html#$j\">$j</a><br>" >> doc/$i.html
		done
		echo "</div>" >> doc/$i.html
	fi

	markdown doc/markdown/$i >> doc/$i.html && echo -e "ok\t$i" || echo -e "fail\t$i"
	cat doc/template/footer.html >> doc/$i.html
done

# build index.html
echo -e "ok\tindex"
cat doc/template/header.html > doc/index.html
cat doc/template/index.html >> doc/index.html
cat doc/template/footer.html >> doc/index.html
