# Local Development

The easiest way to run the site locally is by using the top-level
[Makefile](../Makefile). This has a `site-devel` target and runs a
[Jekyll](https://jekyllrb.com) development server in Docker, which
is useful for checking local documentation changes. By default, the
site will be available on [port 4000](http://127.0.0.1:4000/).

Note that the version of [Jekyll](https://jekyllrb.com) used by the
site is pinned in the [Gemfile](./Gemfile.lock). The `site-devel`
target uses the matching versioned Docker image to ensure compatibility.

Once you are happy with your changes, commit them and push everything
to your fork.  When you submit a PR, Netlify will automatically
generate a preview of your changes.

## Manual Installation

If you need to manually install and run [Jekyll](https://jekyllrb.com)
without using Docker, you can install the [Jekyll](https://jekyllrb.com)
packages on your development system.

On macOS, install the following:

* `brew install rbenv`
* `rbenv install 2.6.3`
* `gem install bundler`

On Ubuntu you will need these packages:

* ruby
* ruby-dev
* ruby-bundler
* build-essential
* zlib1g-dev
* nginx (or apache2)

Install [Jekyll](https://jekyllrb.com) and plug-ins in one fell
swoop. This mirrors the plug-ins used by GitHub Pages on your local
machine including Jekyll, Sass, etc:

* `gem install github-pages`

Then:

1. Clone down your own fork, or clone the main repo `git clone https://github.com/projectcontour/contour` and add your own remote.
2. `cd site`
3. `rbenv local 2.6.3`
4. `bundle install`
5. Serve the site and watch for markup/sass changes `jekyll serve --livereload`. You may need to run `bundle exec jekyll serve --livereload`.
6. View your website at http://127.0.0.1:4000/
