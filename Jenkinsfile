pipeline {
    agent { label 'insights' }
    stages {
        stage('Stage one') {
            steps {
                sh 'echo Hello World'
            }
        }
        stage('Tests') {
            agent {
                docker {
                    image 'registry.access.redhat.com/ubi9/go-toolset:1.18.9'
                    reuseNode true
                }
            }
            steps {
                sh 'ls -lrt'
                sh 'make test'
            }
        }
    }
}
