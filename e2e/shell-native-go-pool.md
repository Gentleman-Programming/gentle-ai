# Native Go service parallelism

Native coordinator and sealed producer re-entries explicitly receive `GOMAXPROCS=1` after environment sanitization. A fresh environment copy replaces any previous assignment; generic service arguments and caller-owned environment slices are unchanged.

This reduces Go execution parallelism, not the number of OS threads. It neither raises nor replaces the existing Tasks64, CPU, memory or deadline limits. Original-installer Node pool controls remain at their existing boundary.

Private service tests verify the coordinator and sealed argument vectors; actual installation must still demonstrate adequate task headroom and successful final publication.