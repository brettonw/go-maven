#! /usr/bin/env perl

use strict;
use warnings FATAL => 'all';
use Cwd;

use File::Which;

# this is a maven wrapper intended to solve the problem that release builds don't actually deploy
# to the local nexus server using the maven release plugin

# global variables
our $gitCommand = which ("git");
our $mavenCommand = which ("mvn") . " --quiet";
our $goPropertiesFileName = "go.properties";
our ($releaseBuildType, $snapshotBuildType) = ("", "-SNAPSHOT");

# function to do what 'chomp' should (but doesn't)
sub trim { my $s = shift; $s =~ s/^\s+|\s+$//g; return $s; };

sub setMavenVersion {
    my $newVersion = shift;
    my $newBuildType = shift;
    my $newVersionCommand = "$mavenCommand versions:set -DnewVersion=$newVersion$newBuildType -DgenerateBackupPoms=false -DprocessDependencies=false --non-recursive";
    #print STDERR "Exec ($versionCommand)\n";
    print STDERR "Setting build version ($newVersion$newBuildType)\n";
    system ($newVersionCommand) && die "Couldn't set version.\n";
}

sub checkin {
    my $message = shift;
    print STDERR "Check-in ($message).\n";
    system ("$gitCommand add --all . && $gitCommand commit -m 'go git ($message)' && $gitCommand push origin HEAD;");
}

sub execute {
    my ($task, $command) = @_;
    print STDERR "Execute task ($task)";
    #print STDERR " as ($command)";
    print STDERR "\n";
    $command = "$command 2>&1 | tee build.txt";
    system ($command) && die ("($task) FAILED\n");
}

# go up until we find a pom file, and then go up as long as there is a parent pom file
while ((getcwd () ne "/") && (! (-f "pom.xml"))) {
    chdir "..";
}
while ((getcwd () ne "/") && (-f "../pom.xml")) {
    chdir "..";
}
if (! (-f "pom.xml")) {
    print STDERR "ERROR - Not a project!\n";
        exit(1);
}
my $directory = getcwd ();
print STDERR "Build in dir ($directory)\n";


# get the version from maven, do this before checking options
my $mvnVersionCommand = "$mavenCommand -Dexec.executable='echo' -Dexec.args='\${project.version}' --non-recursive exec:exec";
my @mvnVersionCommandOutput = `$mvnVersionCommand`;
my $version = trim ($mvnVersionCommandOutput[0]);
$version =~ s/-SNAPSHOT$//;
print STDERR "Build at version ($version)\n";

# allowed options are: [--verbose]? [--clean]? [--notest]? [--git]? [build* | validate | package | install | deploy | release]
# * build is the default command
my $shouldClean = 0;
my $shouldTest = 1;
my $shouldCheckin = 0;
my $task = "build";
my %tasks; $tasks{$_} = $_ for ("build", "validate", "package", "install", "deploy", "release");
foreach (@ARGV) {
    my $arg = lc ($_);
    if ($arg eq "--verbose")  { $mavenCommand =~ s/ --quiet//; }
    elsif ($arg eq "--clean")  { $shouldClean = 1; }
    elsif ($arg eq "--notest") { $shouldTest = 0; }
    elsif ($arg eq "--git") { $shouldCheckin = 1; }
    elsif (exists $tasks{$arg}) { $task = $arg; }
    else { die "Unknown task ($arg).\n"; }
}

# figure out how to fulfill the task
if ($task eq "release") {
        # will be 0 if there are no changes...
        system("$gitCommand diff --quiet HEAD;") && die("Please commit all changes before performing a release.\n");

        # ask the user to supply the new release version (default to the current version sans "SNAPSHOT"
        print "What is the release version (default [$version]): ";
        my $input = <STDIN>;
        $input = trim($input);
        if (length($input) > 0) {$version = $input;}

        # ask the user to supply the next development version (default to a dot-release)
        my ($major, $minor, $dot) = split(/\./, $version);
        my $nextDevelopmentVersion = "$major.$minor." . ($dot + 1);
        print "What will the new development version be (default [$nextDevelopmentVersion]): ";
        $input = <STDIN>;
        $input = trim($input);
        $nextDevelopmentVersion = (length($input) > 0) ? $input : $nextDevelopmentVersion;

        # configure testing by default, belittle the user if they want to skip it
        my $command = $mavenCommand;
        if ($shouldTest == 0) {
            print "WARNING - release without test, type 'y' to confirm (default [n]):";
            $input = <STDIN>;
            $input = lc(trim($input));
            if ($input eq "y") {
                $command = "$command -Dmaven.test.skip=true";
            }
        }

        # set the version, and execute the release deployment build (forced verbose)
        setMavenVersion($version, $releaseBuildType);
        my $releaseCommand = $command;
        $releaseCommand =~ s/ --quiet//;
        execute($task, "$releaseCommand clean deploy");
        checkin("$version");
        print STDERR "Tag release ($version).\n";
        system("$gitCommand tag -a 'Release-$version' -m 'Release-$version';");

        # update the version to the development version and check it in
        setMavenVersion($nextDevelopmentVersion, $snapshotBuildType);
        checkin("$nextDevelopmentVersion");
} else {
    my $command = ($shouldClean == 1) ? "$mavenCommand clean" : "$mavenCommand";
    if ($task eq "build") {
        $command = ($shouldTest == 0) ? "$command compile" : "$command test";
    } else {
        $command = "$command $task";
        if ($shouldTest == 0) { $command = "$command -Dmaven.test.skip=true"; }
    }
    execute ($task, $command);
    if ($shouldCheckin) { checkin("CHECKPOINT - $version$snapshotBuildType"); }
}
