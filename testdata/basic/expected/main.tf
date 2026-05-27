data "aws_ami" "used" {
  most_recent = true
  owners      = ["amazon"]
}
resource "aws_instance" "web" {
  ami = data.aws_ami.used.id
}
