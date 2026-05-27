data "aws_ami" "a" {
  most_recent = true
}
data "aws_vpc" "b" {
  default = true
}
resource "aws_instance" "web" {
  ami    = data.aws_ami.a.id
  vpc_id = data.aws_vpc.b.id
}
