data "aws_vpc" "leaf" {
  default = true
}
data "aws_subnet" "middle" {
  vpc_id = data.aws_vpc.leaf.id
}
data "aws_ami" "used" {
  most_recent = true
}
resource "aws_instance" "web" {
  ami = data.aws_ami.used.id
}
