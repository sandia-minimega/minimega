// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
The present file format

Present files have the following format.  The first non-blank non-comment
line is the title, so the header looks like

	Title of document
	Subtitle of document
	15:04 2 Jan 2006
	Tags: foo, bar, baz
	<blank line>
	Author Name
	Job title, Company
	joe@example.com
	http://url/
	@twitter_name

The subtitle, date, and tags lines are optional.

The date line may be written without a time:

	2 Jan 2006

In this case, the time will be interpreted as 10am UTC on that date.

The tags line is a comma-separated list of tags that may be used to categorize
the document.

The author section may contain a mixture of text, twitter names, and links.
For slide presentations, only the plain text lines will be displayed on the
first slide.

Multiple presenters may be specified, separated by a blank line.

After that come slides/sections, each after a blank line:

  - Title of slide or section (must have asterisk)

    Some Text

    ** Subsection

  - bullets

  - more bullets

  - a bullet with

    *** Sub-subsection

    Some More text

    Preformatted text
    is indented (however you like)

    Further Text, including invocations like:

    .code x.go /^func main/,/^}/
    .play y.go
    .image image.jpg
    .iframe http://foo
    .link http://foo label
    .html file.html
    .caption _Gopher_ by [[http://www.reneefrench.com][Renée French]]

    Again, more text

Blank lines are OK (not mandatory) after the title and after the
text.  Text, bullets, and .code etc. are all optional; title is
not.

Lines starting with # in column 1 are commentary.

Fonts:

Within the input for plain text or lists, text bracketed by font
markers will be presented in italic, bold, or program font.
Marker characters are _ (italic), * (bold) and ` (program font).
Unmatched markers appear as plain text.
Within marked text, a single marker character becomes a space
and a doubled single marker quotes the marker character.

	_italic_
	*bold*
	`program`
	_this_is_all_italic_
	_Why_use_scoped__ptr_? Use plain ***ptr* instead.

Inline links:

Links can be included in any text with the form [[url][label]], or
[[url]] to use the URL itself as the label.

Functions:

A number of template functions are available through invocations
in the input text. Each such invocation contains a period as the
first character on the line, followed immediately by the name of
the function, followed by any arguments. A typical invocation might
be

	.play demo.go /^func show/,/^}/

(except that the ".play" must be at the beginning of the line and
not be indented like this.)

Here follows a description of the functions:

mega:

Load a minimega script:

	.mega foo.mm

You can specify an optional integer height for the mega script, in case it's
too large to render:

	.mega foo.mm 300

link:

Create a hyperlink. The syntax is 1 or 2 space-separated arguments.
The first argument is always the HTTP URL.  If there is a second
argument, it is the text label to display for this link.

	.link http://golang.org golang.org

image:

The template uses the function "image" to inject picture files.

The syntax is simple: 1 or 3 space-separated arguments.
The first argument is always the file name.
If there are more arguments, they are the height and width;
both must be present, or substituted with an underscore.
Replacing a dimension argument with the underscore parameter
preserves the aspect ratio of the image when scaling.

	.image images/betsy.jpg 100 200

	.image images/janet.jpg _ 300

caption:

The template uses the function "caption" to inject figure captions.

The text after ".caption" is embedded in a figcaption element after
processing styling and links as in standard text lines.

	.caption _Gopher_ by [[http://www.reneefrench.com][Renée French]]

iframe:

The function "iframe" injects iframes (pages inside pages).
Its syntax is the same as that of image.

html:

The function html includes the contents of the specified file as
unescaped HTML. This is useful for including custom HTML elements
that cannot be created using only the slide format.
It is your responsibilty to make sure the included HTML is valid and safe.

	.html file.html
*/
package present
