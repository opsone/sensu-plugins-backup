#! /usr/bin/env ruby
# frozen_string_literal: true

require 'dotenv'
require 'open3'
require 'sensu-plugin/check/cli'

class CheckBackupRestic < Sensu::Plugin::Check::CLI
  option :env_file,
         short: '-e FILE',
         long: '--env-file FILE',
         description: 'Path to restic environment file to source (default: /etc/restic/env)',
         default: '/etc/restic/env'

  option :restic_binary,
         short: '-b PATH',
         long: '--restic-binary PATH',
         description: 'Path to the restic binary (default: restic)',
         default: 'restic'

  option :read_data,
         long: '--read-data',
         description: 'Read all data blobs during check (thorough but slow — use at low frequency)',
         boolean: true,
         default: false

  option :read_data_subset,
         long: '--read-data-subset SUBSET',
         description: 'Read a subset of data blobs: "n/t" (e.g. "1/7") or "x%" (e.g. "10%")',
         default: nil

  SUBSET_PATTERN = %r{\A(\d+/\d+|\d+(\.\d+)?%)\z}

  def load_env_file
    Dotenv.overload(config[:env_file])
  rescue Errno::ENOENT
    critical "Environment file not found: #{config[:env_file]}"
  rescue Errno::EACCES
    critical "Cannot read environment file: #{config[:env_file]}"
  end

  def check_mode
    if config[:read_data]
      'FULL'
    elsif config[:read_data_subset]
      "SUBSET #{config[:read_data_subset]}"
    else
      'QUICK'
    end
  end

  def build_cmd
    cmd = [config[:restic_binary], 'check']

    if config[:read_data] && config[:read_data_subset]
      critical '--read-data and --read-data-subset are mutually exclusive'
    elsif config[:read_data]
      cmd << '--read-data'
    elsif config[:read_data_subset]
      subset = config[:read_data_subset]
      critical "Invalid --read-data-subset format: #{subset.inspect} (expected n/t or x%)" unless SUBSET_PATTERN.match?(subset)
      cmd << "--read-data-subset=#{subset}"
    end

    cmd
  end

  def run
    load_env_file

    stdout, stderr, status = Open3.capture3(*build_cmd)
    output = [stdout, stderr].reject(&:empty?).join("\n").strip
    mode = check_mode

    if status.success?
      ok "[#{mode}] #{output.lines.last&.chomp || "restic check passed"}"
    else
      critical "[#{mode}] restic check failed: #{output}"
    end
  end
end
