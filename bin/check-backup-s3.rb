#! /usr/bin/env ruby
# frozen_string_literal: true

require 'aws-sdk-s3'
require 'sensu-plugin/check/cli'

class CheckBackupS3 < Sensu::Plugin::Check::CLI
  option :s3_access_key,
         short: '-a S3_ACCESS_KEY',
         long: '--s3-access-key S3_ACCESS_KEY',
         description: "S3 Access Key. Either set ENV['AWS_ACCESS_KEY'] or provide it as an option",
         default: ENV['AWS_ACCESS_KEY']

  option :s3_secret_key,
         short: '-k S3_SECRET_KEY',
         long: '--s3-secret-key S3_SECRET_KEY',
         description: "S3 Secret Secret Key. Either set ENV['AWS_SECRET_KEY'] or provide it as an option",
         default: ENV['AWS_SECRET_KEY']

  option :s3_region,
         short: '-r S3_REGION',
         long: '--s3-region REGION',
         description: 'S3 Region (defaults to eu-west-3).',
         default: 'eu-west-3'

  option :s3_endpoint,
         short: '-e S3_ENDPOINT',
         long: '--s3-endpoint ENDPOINT',
         description: 'S3 Endpoint (defaults to https://s3.eu-west-3.amazonaws.com).',
         default: 'https://s3.eu-west-3.amazonaws.com'

  option :bucket_name,
         short: '-b BUCKET_NAME',
         long: '--bucket-name',
         description: 'The name of the S3 bucket where object lives'.dup,
         required: true

  option :directory_name,
         short: '-d DIR_NAME',
         long: '--directory-name',
         description: 'The name of directory to check (defaults to /var/archives)',
         default: '/var/archives'

  def s3_config
    {
      access_key_id: config[:s3_access_key],
      secret_access_key: config[:s3_secret_key],
      region: config[:s3_region],
      endpoint: config[:s3_endpoint]
    }
  end

  def run
    s3 = Aws::S3::Client.new(s3_config)

    Dir.glob("#{config[:directory_name]}/*") do |file|
      next if File.extname(file) == '.md5'

      file_name = File.basename(file)
      file_size = File.size(file)

      output      = s3.head_object(bucket: config[:bucket_name], key: file_name)
      remote_size = output[:content_length]

      critical "S3 object #{file_name} size: #{file_size} bytes (bucket #{remote_size})" if file_size != remote_size
    rescue Aws::S3::Errors::NotFound
      critical "S3 object #{file_name} not found in bucket #{config[:bucket_name]}"
    end

    ok "All files in #{config[:directory_name]} exists in S3 bucket #{config[:bucket_name]}"
  rescue Aws::S3::Errors::ServiceError => e
    critical "S3 exception - #{e.message} - #{e.backtrace}"
  end
end
