from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
import os

os.chdir('/home/pi/creality-remote-control')
server = ThreadingHTTPServer(('0.0.0.0', 8080), SimpleHTTPRequestHandler)
server.serve_forever()
