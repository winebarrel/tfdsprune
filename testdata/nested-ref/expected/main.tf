data "aws_ami" "used" {
  most_recent = true
}
resource "aws_instance" "web" {
  ami = data.aws_ami.used.id
  network_interface {
    subnet_id = "subnet-${data.aws_ami.used.architecture}"
  }
}
