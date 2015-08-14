# chug

Generates a Markdown changelog.

## Usage

`chug -o bjyoungblood -r chug -token MY_GITHUB_API_TOKEN`

You will be prompted for the start and end refs, and you will be given either some errors or some Markdown to paste into your changelog.

Your Github API token can also be read from the environment (`GITHUB_API_TOKEN`).

## Goals

This project aims to:

 - Print a list of issues referenced by commits between a start and end ref
 - Be simple and easy to use

## Non-goals

This project will not:

 - Manage your entire changelog for you
 - Manage versioning for you
 - Manage releases for you
 - Determine which items to include in the changelog from a non-git source

### This already exists

I know.

This tool offers some benefits (at least to me) over other tools I've seen:

 - Reads issue numbers directly from commit messages, rather than relying on merge/close dates from the Github API
   - This enables you to generate a changelog for any git ref (tag, commit, branch, etc.)
 - Less reliance on the Github API than other tools
   - This tool currently uses the Github API to pull issue titles and links, but because the core uses `libgit2` to read git data, it could be made to use a different platform with relatively little modification
 - Requires you to put issue numbers in your commits
   - Why wouldn't you be doing this anyway? ;)
 - Other tools are not written in Go
