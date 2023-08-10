#! /usr/bin/env ruby
# frozen_string_literal: true

require 'net/ftp'
require 'sensu-plugin/check/cli'

class CheckBackupFtp < Sensu::Plugin::Check::CLI
  option :ftp_user,
         short: '-u USER',
         long: '--ftp-user USER',
         description: 'FTP User'

  option :ftp_password,
         short: '-p PASS',
         long: '--ftp-password PASS',
         description: 'FTP Password'

  option :ftp_host,
         short: '-h HOST',
         long: '--ftp-host HOST',
         description: 'FTP Hostname'.dup,
         required: true

  option :ftp_directory,
         short: '-D DIR_NAME',
         long: '--ftp-directory DIR_NAME',
         description: 'FTP directory'

  option :directory_name,
         short: '-d DIR_NAME',
         long: '--directory-name DIR_NAME',
         description: 'The name of directory to check',
         default: '/var/archives'

  def run
    ftp = Net::FTP.new(config[:ftp_host])
    ftp.login(config[:ftp_user], config[:ftp_password]) if config[:ftp_user] && config[:ftp_password]
    ftp.chdir(config[:ftp_directory]) if config[:ftp_directory]
    ftp.binary = true

    Dir.glob("#{config[:directory_name]}/*") do |file|
      next if File.extname(file) == '.md5' || File.directory?(file)

      file_name = File.basename(file)
      file_size = File.size(file)

      if ftp.ls(file_name).empty?
        critical "FTP file #{file_name} missing"
      elsif file_size != ftp.size(file_name)
        critical "FTP file #{file_name} size differ from original"
      end
    end

    ok "All files in #{config[:directory_name]} exists on FTP #{config[:ftp_host]}"
  rescue Net::FTPError => e
    critical "FTP exception - #{e.message}"
  ensure
    ftp&.quit
  end
end
