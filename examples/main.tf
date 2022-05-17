terraform {
  required_providers {
    fly = {
      source = "dov.dev/fly/fly-provider"
    }
  }
}

resource "fly_app" "exampleApp" {
  name    = "hellofromterraform"
  regions = ["ewr", "ord", "lax", "mia"]
}

resource "fly_volume" "exampleVol" {
  name       = "exampleVolume"
  app        = "hellofromterraform"
  size       = 10
  region     = "ewr"
  depends_on = [fly_app.exampleApp]
}

resource "fly_ip" "exampleIp" {
  app        = "hellofromterraform"
  type       = "v4"
  depends_on = [fly_app.exampleApp]
}

resource "fly_ip" "exampleIpv6" {
  app        = "hellofromterraform"
  type       = "v6"
  depends_on = [fly_ip.exampleIp]
}

resource "fly_cert" "exampleCert" {
  app        = "hellofromterraform"
  hostname   = "example.com"
  depends_on = [fly_app.exampleApp]
}

# Example of using resource value. Can be used to, for example set dns with other providers.
output "testipv4" {
  value = fly_ip.exampleIp.address
}

output "testipv6" {
  value = fly_ip.exampleIpv6.address
}
