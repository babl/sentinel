FROM busybox
ADD sentinel_linux_amd64 /bin/sentinel
ADD babl_linux_amd64 /bin/babl
CMD ["/bin/sentinel"]