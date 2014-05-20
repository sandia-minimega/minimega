#!/bin/bash

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

	markdown doc/markdown/$i >> doc/$i.html
	cat doc/template/footer.html >> doc/$i.html
done
