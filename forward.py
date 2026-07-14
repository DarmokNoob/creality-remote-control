import adbutils

adb = adbutils.AdbClient()
devices = adb.device_list()
print("Devices found:", devices)

d = devices[0]
d.forward("tcp:18188", "tcp:18188")
print("Forwarding localhost:18188 -> device:18188. Leave this running.")

import time
while True:
    time.sleep(60)
