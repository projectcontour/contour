Fix HTTPProxy IP filter policies being partially applied when an invalid CIDR is followed by a valid one. The virtual host or route is now rejected, the same as when the last CIDR is invalid.
