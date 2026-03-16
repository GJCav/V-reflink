# Feedbacks

**Enhancement**

`vreflinkd` should reject starting when the `VREFLINK_SHARE_ROOT` is not a valid root. "valid" here means: 

1. this path exists;
2. the filesystem of this path support `reflink`.

**Enhancement**

`vreflink` should resolve the `src` and `dst` path relative to the working directory when they are relative paths. 
Currently, `vreflink` requires users to type absolute path, which is not convenient.

**Bug**

The `dst` file created by the daemon is owned by `root`, not the user from the guest that invoked the 
request. The expected behavior is to make the owner user & group to be that of the request invoker.
This may change the API and the security strategy. But must be solved to support daily operations.

**Discuss**

There should be an elegant solution to support multiple `VREFLINK_SHARE_ROOT`. A simple proposal is to 
start an individual daemon process for a root and distinguish each with the VSOCK port. This simply works
but we welcome discussion about other solutions.


