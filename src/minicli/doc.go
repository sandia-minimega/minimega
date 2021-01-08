// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

// The minicli package implements a simple command line interface for minimega.
// During startup, minimega initializers will register callbacks with minicli.
// Each registration consists of a pattern that the user's input should match,
// and a function pointer that should be invoked when there's a match.
//
// Patterns consist of required text, required and optional fields, multiple
// choice arguments, and variable number of arguments. The pattern syntax is as
// follows:
//
//  foo bar     literal required text, as in "capture netflow"
//  <foo>       a required string, returned in the arg map with key "foo"
//  <foo bar>   a required string, still returned in the arg map with key "foo".
//              The extra text is just documentation
//  <foo,bar>   a required multiple choice argument. Returned as whichever choice
//              is made in the argmap (the argmap key is simply created).
//  [foo]       an optional string, returned in the arg map with key "foo".
//              There can be only one optional arg and it must be at the end of
//              the pattern.
//  [foo,bar]   an optional multiple choice argument. Must be at the end of
//              pattern.
//  <foo>...    a required list of strings, one or more, with the key "foo" in
//              the argmap. Must be at the end of the pattern.
//  [foo]...    an optional list of strings, zero or more, with the key "foo" in
//              the argmap. This is the only way to support multiple optional
//              fields. Must be at the end of the pattern.
//  (foo)       a nested subcommand consuming all items to the end of the input
//              string. Must be at the end of pattern.
//
// minicli also supports multiple output rendering modes and stream and tabular
// compression.
package minicli
