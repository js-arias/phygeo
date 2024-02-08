# PhyGeo

`PhyGeo` is a tool for phylogenetic biogeography analysis.

## Installing

There are two ways to install *PhyGeo*.
If you are only interested in the command-line tool,
just go to the [Releases tab](https://github.com/js-arias/phygeo/releases),
select the last release,
choose an executable for your system architecture,
renamed at your will
(Here,
it is assumed that you use the name `phygeo`
or `phygeo.exe` if you use  Windows),
and put it in your default `bin` directory.
If you want an up-to-date tool
you require the [go tool](https://go.dev/dl/)
and install the *PhyGeo* package by running:

```bash
go install github.com/js-arias/phygeo@latest
```

If you want to use the package
in your own code,
just import the package,
for example:

```go
import "github.com/js-arias/phygeo/infer/diffusion"
```

## Usage

*PhyGeo* is a command-line tool
formed by a set of commands.
Many commands have their own sub-commands.
To see the list of commands,
just type the name of the application:

```bash
phygeo
```

The best way to learn about the commands
is by reading the included on-line help,
using the command `help`,
and using the command of interest as a parameter:

```bash
phygeo help diff map
```

This [simple example dataset](https://github.com/js-arias/schistanthe-data),
which includes the instructions to run it,
will be helpful to start using the program.

### Setting a *PhyGeo* project

A diffusion analysis with *PhyGeo*
requires three data sources:
a phylogenetic tree,
the distribution range of the terminals,
and a paleogeography model.
These data sources are stored in a *project file*,
so you don't have to define them every time.
They also give you the possibility
of having multiple projects
based on slightly different data
(for example,
the same phylogenetic tree
and distribution ranges,
but a different paleogeographic model).

Maybe the best way to start a project
is by setting the paleogeography model:

```bash
phygeo geo add --type geomotion project.tab muller-2022-motion.tab
phygeo geo add --type landscape project.tab cao-2017-landscape.tab
```

In this example,
a plate motion model called `muller-2022-motion.tab`
and a landscape model called `cao-2017-landscape.tab`
are added to `project.tab`.
As it is possible that `project.tab` does not exist,
it will be created automatically in the first call.

As paleogeography models
are quite specialized datasets,
[here is a repository](https://github.com/js-arias/geomodels)
with several models ready to be used with *PhyGeo*.

To define the priors of the pixels,
you must define a pixel prior file
([here is an example](https://github.com/js-arias/schistanthe-data/blob/main/model-pix-prior.tab)
of this kind of file)
and then add it to the project:

```bash
phygeo geo prior --add model-pix-prior.tab project.tab
```

Phylogenetic trees in *PhyGeo* must be time-calibrated.
The trees must be fully dichotomous.
The preferred tree file is a tab-delimited file
([here is an example file](https://github.com/js-arias/schistanthe-data/blob/main/rhodo-tree-360.tab)).
To add a tree file,
you must define the name of the destination file
(if it is the first tree to be added),
the project file,
and one or more files
with the trees
(usually a single file):

```bash
phygeo tree add -f data-tree.tab project.tab vireya-tree.tab
```

Most users usually have the trees in [newick format](https://en.wikipedia.org/wiki/Newick_format).
To import a newick tree,
use the flag --newick
with the name of the added tree:

```bash
phygeo tree add -f data-tree.tab --newick vireya project.tab vireya.tre
```

See the documentation of the command `tree add`
for more information about adding trees.

Specimen records
are stored as a set of pixels
(presence-absence pixels)
or range maps.
Both files have the same format
([here is an example file](https://github.com/js-arias/schistanthe-data/blob/main/rhodo-points-360.tab)).
To import a set of records to a project,
you must define a destination file
(if it is the first set of records to be added),
the kind of file
(points or ranges),
the project file,
and one or more files with the records:

```bash
phygeo range add -f data-points.tab project.tab vireya-points.tab pseudovireya-points.tab
```

It is possible that your file with specimen records
is a table with latitudes and longitudes
or a table downloaded from GBIF.
In such cases,
you can import them using the flag --format:

```bash
phygeo range add -f data-points.tab --format darwin project.tab gbif-download.csv 
```

See the documentation of the command `range add`
for more information
about adding specimen records.

Note that the pixelation used
for the specimen records
must be of the same resolution
as the paleogeography models.

To be sure that all terminals
in all the trees
in the project have at least
a single valid record,
use:

```bash
phygeo range taxa --val project.tab
```

if there is no problem,
the command will finish silently;
otherwise,
it will report
the name of the terminals
without geographic data.

### Analyzing the data

With a valid project,
it is possible to make inferences from the data.
There are several possibilities.
Maybe the most simple
is to just attempt a likelihood estimation
of the data with an a priori *lambda* value,
just to see what happens,
or because from a previous analysis,
you know that the given lambda
is the maximum likelihood value:

```bash
phygeo diff like --lambda 100 -o like project.tab
```

This analysis will create a new file
with the prefix `like`
(for example, `like-project.tab-vireya.100.000000-down.tab`),
which contains the down-pass likelihoods
(i.e., the conditional likelihoods)
of each pixel in each node,
so it is a large file.
As the calculation of the likelihood conditionals
is a time-consuming operation,
this file will be helpful
to skip that operation in further analysis.
For example,
it can be used to perform stochastic mapping:

```bash
phygeo diff particles -i like-project.tab-vireya-100.000000-down.tab -o p project.tab
```

This analysis
will create a new file with the prefix `p`
(for example,
'p-vireya-100.000000x1000.tab';
[here is an example file](https://github.com/js-arias/schistanthe-data/blob/main/ml-project-360.tab-vireya-150.000000x1000.tab).
This file will contain all the simulated dispersal paths,
so it is usually a large file.

As you probably want to know
the maximum likelihood estimate of *lambda*,
you can use the command `diff ml`:

```bash
phygeo diff ml project.tab
```

The maximum likelihood estimation
will be printed on the screen.
It uses a simple hill-climbing algorithm
that stops by default
when the step size is smaller than 1.0;
you can set a more detailed bound
(but with a larger execution time).

Maybe
you prefer a Bayesian analysis.
As the only free parameter is the *lambda* value,
you can make a simple integration:

```bash
phygeo diff integrate --min 100 --max 300 --parts 500 project.tab > log-like.tab
```

and then,
using any program to read tab-delimited data
(in this case `log-like.tab`,
[here is an example file](https://github.com/js-arias/schistanthe-data/blob/main/vireya-integrate-360.tab)),
you can provide the prior for *lambda*
(or just use the integration output,
assuming a flat uniform prior).

To sample from the posterior
(or for any distribution),
you can use the same `diff integrate` command,
but define a sampling function
(at the moment,
it just implements the [gamma distribution](https://en.wikipedia.org/wiki/Gamma_distribution)):

```bash
phygeo diff integrate --distribution "gamma=75,0.5" -p 100 --parts 1000 project.tab
```

In this execution,
for each sample
(it will make 1000,
defined with the flag `--parts`),
it will make 100 stochastic mappings
(defined with the flag `-p`).
The output will have the prefix
`sample`
(for example,
`sample.tab-project.tab-vireya-sampling-1000x100.tab`).
These files are usually large
and are of the same format
as the output files produced
by the `diff particles` command.

### Working with the output

The results of the `diff particles` command,
or `diff integrate --distribution` command,
form the most important output of the program.
These files contain one
or more stochastic mappings
(usually more than 100),
i.e.,
the pixel locations of the nodes
and internodes
(branches that cross a time stage
defined by the paleogeography model).

We can transform the stochastic maps
into pixel frequencies,
which are the approximation
of the pixel posterior at each node.
These frequencies can be raw
(i.e., just counts of sampled pixels)
or smoothed using a spherical KDE:

```bash
phygeo diff freq --kde 1000 -i p-vireya-100.000000x1000.tab -o kde project.tab
```

In this example,
the pixel frequencies
will be stored in a file with the `kde` prefix
(for example,
`kde-project.tab-p-vireya-100.000000x1000.tab.tab`),
which is usually large.

Then we can create an image map of the frequencies:

```bash
phygeo diff map -c 1440 --key landscape-key.tab --gray -i kde-project.tab-p-vireya-100.000000x1000.tab.tab -o "ml-95/ml-95" project.tab
```

The command `diff map`
will create a reconstruction
from the frequency file
using a [rainbow color scheme](https://personal.sron.nl/~pault/#fig:scheme_rainbow_smooth)
(from blue for pixels with low posterior probability
to red for pixels with a high posterior);
see [this directory](https://github.com/js-arias/schistanthe-data/tree/main/recs-95)
for an example output.
The command `diff map` has a lot of output options;
see the command help for more information.
Here are some options:
to produce rotated
(the default)
or unrotated maps
(maps with current geographic locations,
`--unrot` flag),
to output each node
(the default),
or output by time stage
(`--richness` flag).
A key file
([here is an example](https://github.com/js-arias/schistanthe-data/blob/main/landscape-key.tab))
can be used to define the colors
for the background geography
(with the flag `--gray`,
it will use a grey scale).

As stochastic maps
include the starting and ending pixel
at each node,
it is possible to measure
the distance traveled by a particle
and its speed.
Use the command `diff speed`
to retrieve general speed results:

```bash
phygeo diff speed --tree speed --step 5 --box 5 -i ml-project.tab project.tab > speed.txt
```

This example
produces a tree
with the speed values colored
in a [rainbow color scheme](https://personal.sron.nl/~pault/#fig:scheme_rainbow_smooth)
(faster lineages in red,
slower in blue,
see [this example file](https://github.com/js-arias/sapindaceae/blob/main/speed-joyce2023.svg)),
in an svg format,
and a log file
([here is an example file](https://github.com/js-arias/sapindaceae/blob/main/speed-branch.txt)),
with the speed
and distance traveled on average
for each node.
With this command,
it is also possible
to measure the speed
at different time stages
(using the flag `--time`).
Consult the `help diff speed` command
to learn more about this command.

## Citation

Here is the suggested citation for the *PhyGeo* tool:

Arias, J.S. (2023)
*PhyGeo: a tool for phylogenetic biogeography*.
(available at: <https://github.com/js-arias/phygeo>).
doi:[10.5281/zenodo.10636373](https://doi.org/10.5281/zenodo.10636373).

The description of the method is available as a pre-print:

Arias, J.S. (2023)
Phylogenetic biogeography inference using dynamic paleogeography models and explicit geographic ranges.
*BioRXiv* 2023.11.16.567427.
doi:[10.1101/2023.11.16.567427](https://doi.org/10.1101/2023.11.16.567427).

See the manuscript
for more information about previous proposals for phylogenetic biogeography methods
using explicit geographic ranges
or the diffusion model.

## Additional resources

- [Paleogeographic models for *PhyGeo*](https://github.com/js-arias/geomodels)
- Sample *PhyGeo* datasets:
  - [*Schistanthe* section of *Vireya*](https://github.com/js-arias/schistanthe-data/).
  - [Sapindaceae](https://github.com/js-arias/sapindaceae).
- Other tools:
  - [GBIFer](https://github.com/js-arias/gbifer):
    manipulate GBIF tables.
  - [Plates](https://github.com/js-arias/earth):
    manipulate paleogeographic models.
  - [TaxRange](https://github.com/js-arias/ranges):
    manipulate pixelated distribution data.
  - [TimeTree](https://github.com/js-arias/timetree):
    manipulate time calibrated trees.

## Contribution and bug reports

The best way to contribute to the package
is by running the program,
detecting bugs,
or asking for features.
Use the tab [issues](https://github.com/js-arias/phygeo/issues)
to file a bug
or ask for a feature.

If you like programming,
you can create tools and packages to import
export,
or analyze data and results to or from *PhyGeo*.
If you send me the link,
I will post the link of your tool
or package.

Of course,
this package is open source,
so you can modify it at your will!

## Authorship and license

Copyright © 2023 J. Salvador Arias <jsalarias@gmail.com>.
All rights reserved.
Distributed under BSD2 licenses that can be found in the LICENSE file.
