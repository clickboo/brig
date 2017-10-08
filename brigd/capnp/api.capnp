using Go = import "/go.capnp";

@0xea883e7d5248d81b;
$Go.package("capnp");
$Go.import("github.com/disorganizer/brig/brigd/capnp");

struct StatInfo $Go.doc("StatInfo is a stat-like description of any node") {
    path    @0 :Text;
    hash    @1 :Data;
    size    @2 :UInt64;
    inode   @3 :UInt64;
    isDir   @4 :Bool;
    depth   @5 :Int32;
    modTime @6 :Text;
}

interface FS {
    stage @0 (localPath :Text, repoPath :Text);
    list  @1 (root :Text, maxDepth :Int32) -> (entries :List(StatInfo));
    cat   @2 (path :Text) -> (fifoPath :Text);
    mkdir @3 (path :Text, createParents :Bool);
}

interface VCS {
}

interface Meta {
    quit @0 ();
    ping @1 () -> (reply :Text);
    init @2 (basePath :Text, owner :Text, backend :Text);
}

# Group all interfaces together in one API object,
# because apparently we have this limitation what one interface
# more or less equals one connection.
interface API extends(FS, VCS, Meta) {
    version @0 () -> (version :Int32);
}
