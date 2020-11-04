# MikroTiChestra
There is a beeper in MikroTik routers… You know what I mean.

## Usage

1. Build the project using
```
$ go get -u -v
$ go build
```

2. Copy the example configuration file
```
$ cp MikroTiChestra.conf{.example,}
```

3. Edit the configuration file

4. Create some MIDI files using your favorite DAW software

5. SSH into your routers at least once to ensure `~/.ssh/known_hosts` contains public keys of your routers, this is for security

6. Party on!
```
$ ./MikroTiChestra super_mario_bros_overworld.mid never_gonna_give_you_up.mid
```

## License

This program is released under the MIT license, please refer to [LICENSE](LICENSE) for legal stuff.

This program is released for the hope that it may be helpful and comes with **absolutely no warranty**. For example, I am not responsible for your router's [H.C.F.](https://en.wikipedia.org/wiki/Halt_and_Catch_Fire_(computing)), your neighbor's complaint, or any possible damage to your hearing, or any lawsuit you received because you played a copyrighted song on a deserted island, or perhaps a hurricane caused by the butterfly effect. If you point at me, I will laugh at you.
