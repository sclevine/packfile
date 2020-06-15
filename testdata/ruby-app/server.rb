# Requires the Gemfile
require 'bundler' ; Bundler.require

# By default Sinatra will return the string as the response.
get '/hello-world' do
  "Hello World!"
end
