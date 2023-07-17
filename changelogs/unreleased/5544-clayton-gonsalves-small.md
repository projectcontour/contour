## Fix order of Global ExtAuth and Global Ratelimit

This ensures that the order of execution of extauth and global ratelimit is the same across HTTP and HTTPS virtualhosts, which is Auth goes first then Global ratelimit.

